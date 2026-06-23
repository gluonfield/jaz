package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/jaztools"
	"github.com/wins/jaz/backend/internal/loops"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/pathsafe"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/skills"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/terminal"
	"github.com/wins/jaz/backend/internal/threads"
	"github.com/wins/jaz/backend/internal/voice"
	"github.com/wins/jaz/backend/internal/widgets"
)

type ACPManager interface {
	CreateSession(context.Context, acp.SpawnRequest) (storage.Session, error)
	Spawn(context.Context, acp.SpawnRequest) (acp.SpawnResult, error)
	Send(context.Context, acp.SendRequest) (acp.Job, error)
	Steer(context.Context, acp.SteerRequest) (acp.Job, error)
	SendSideChat(context.Context, acp.SideChatRequest) error
	Status(string) (acp.Job, error)
	List() []acp.Job
	RunUtilityPrompt(context.Context, acp.UtilityPromptRequest) (string, error)
	Agents() []string
	AnswerInteractive(context.Context, acp.InteractiveAnswer) error
	Cancel(context.Context, string) (acp.Job, error)
}

type MCPRuntime interface {
	Refresh(context.Context)
	Status(string) mcpconfig.ServerStatus
	Test(context.Context, mcpconfig.Server) mcpconfig.ServerStatus
	Authorize(context.Context, mcpconfig.Server) mcpconfig.ServerStatus
}

type Server struct {
	Agent                *agent.Agent
	Store                storage.Store
	Routes               Routes
	ACP                  ACPManager
	MCP                  MCPRuntime
	Locks                *sessionlock.Locks
	Events               *sessionevents.Bus
	Loops                *loops.Service
	Threads              *threads.Service
	Widgets              *widgets.Service
	STT                  voice.STT
	TTS                  voice.TTS
	ModelProviderRuntime provider.ReloadableProvider
	// Providers is the live registry of effective model providers (catalog +
	// application.yaml + DB customs). Read it through modelProviders().
	Providers    provider.Source
	AgentCatalog acp.AgentCatalog
	AuthKey      string
	// Prompts derives the system prompt fresh per turn from disk, so skill
	// and prompt-file edits apply without a restart.
	Prompts *coordinator.Builder
	Root    string
	// Workspace is the directory sessions run within; the new-thread directory
	// picker browses it (confined by pathsafe).
	Workspace string
	Log       *log.Logger

	// Memory owns the embedded jazmem instance, its enabled gate, scheduler,
	// and MCP surface.
	Memory   *memoryservice.Service
	JazTools *jaztools.Service

	Terminal     *terminal.Manager
	Devices      *deviceauth.Service
	terminalOnce sync.Once

	acpAuthLoginJobs sync.Map
	worktreePruneMu  sync.Mutex
}

type Route struct {
	Pattern string
	Handler http.Handler
}

type Routes []Route

func (s *Server) logger() *log.Logger {
	if s.Log != nil {
		return s.Log
	}
	return log.Default()
}

type messageRecordStore interface {
	LoadMessageRecords(string) ([]storage.Message, error)
}

type reasoningMessageStore interface {
	SaveMessagesWithReasoning(string, []provider.Message, map[int]string) error
}

type mediaReasoningMessageStore interface {
	SaveMessagesWithReasoningAndMedia(string, []provider.Message, map[int]string, map[string][]media.Ref) error
}

type usageStore interface {
	AddUsage(string, storage.Usage) error
}

func (s *Server) resolveWorkspaceDir(directory string) (string, error) {
	if strings.TrimSpace(s.Workspace) == "" {
		return "", fmt.Errorf("workspace is not configured")
	}
	cwd, err := pathsafe.Resolve(s.Workspace, directory)
	if err != nil {
		return "", err
	}
	return cwd, os.MkdirAll(cwd, 0o755)
}

func (s *Server) handleListACPAgents(w http.ResponseWriter, r *http.Request) {
	agents := []string{}
	if s.ACP != nil {
		agents = s.ACP.Agents()
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// Caps for the @-mention file index: enough to cover a working tree's
// surface without shipping a monorepo over the wire.
const workspaceFileIndexMaxEntries = 5000
const workspaceFileIndexMaxDepth = 8

// handleListWorkspaceFiles returns a file/directory index of a session working
// directory for the composer's @-mention picker. Relative paths stay confined
// to the workspace; absolute paths are server-side project directories. Inside
// a git repository the index follows .gitignore (tracked + untracked files via
// git ls-files); elsewhere it falls back to a breadth-first walk that skips
// dotfiles and dependency/build directories. The echoed absolute root lets the
// client expand tagged entries to full paths.
func (s *Server) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	abs, err := s.resolveWorkspaceFileRoot(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var entries []pathsafe.Entry
	var truncated bool
	if files, gitErr := gitinfo.ListFiles(r.Context(), abs); gitErr == nil {
		entries, truncated = pathsafe.EntriesFromFiles(files, workspaceFileIndexMaxEntries)
	} else if entries, truncated, err = pathsafe.Tree(abs, workspaceFileIndexMaxDepth, workspaceFileIndexMaxEntries); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"root": abs, "entries": entries, "truncated": truncated})
}

// handleListSkills lists the skill catalog for the composer's $-mention picker.
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	var (
		catalog skills.Catalog
		err     error
	)
	if r.URL.Query().Has("path") {
		cwd, resolveErr := s.resolveWorkspaceFileRoot(r.URL.Query().Get("path"))
		if resolveErr != nil {
			writeError(w, http.StatusBadRequest, resolveErr)
			return
		}
		catalog, err = skills.LoadForWorkspace(s.Root, cwd)
	} else {
		catalog, err = skills.Load(s.Root)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": catalog.Skills})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("last") == "true" {
		session, err := s.Store.LastRootSession()
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, canonicalSessionResponse(session))
		return
	}
	limit := 0
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &limit)
	}
	filter := storage.SessionFilter{
		ParentID:        query.Get("parent_id"),
		ParentOnly:      query.Has("parent_id"),
		RootOnly:        query.Get("root") == "true",
		Runtime:         query.Get("runtime"),
		IncludeChildren: query.Get("include_children") == "true",
		SourceType:      query.Get("source_type"),
		SourceID:        query.Get("source_id"),
		IncludeSourced:  query.Get("include_sourced") == "true",
		Archived:        query.Get("archived") == "true",
		Limit:           limit,
	}
	if raw := strings.TrimSpace(query.Get("updated_since")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("updated_since must be RFC3339: %w", err))
			return
		}
		filter.UpdatedSince = parsed
	}
	sessions, err := s.Store.ListSessions(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": canonicalSessionResponses(sessions)})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	sessionRef, action, hasAction := strings.Cut(rest, "/")
	if sessionRef == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	session, err := s.Store.LoadSession(sessionRef)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if !hasAction {
		writeJSON(w, http.StatusOK, canonicalSessionResponse(session))
		return
	}
	switch action {
	case "messages":
		s.writeSessionMessages(w, session)
	case "events":
		s.streamSessionEvents(w, r, session.ID)
	case "transcript":
		s.writeSessionTranscript(w, r, session)
	case "repo":
		s.handleSessionRepo(w, r, session)
	case "repo/changes":
		s.handleSessionRepoChanges(w, r, session)
	case "repo/diff":
		s.handleSessionRepoDiff(w, r, session)
	case "file":
		s.handleSessionFile(w, r, session)
	case "terminal":
		s.handleSessionTerminal(w, r, session)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

// writeSessionMessages serves the thread page's full hydration payload:
// persisted messages, activity, transcript events, and ACP state.
func (s *Server) writeSessionMessages(w http.ResponseWriter, session storage.Session) {
	var messages any
	if recordStore, ok := s.Store.(messageRecordStore); ok {
		records, err := recordStore.LoadMessageRecords(session.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		messages = messageRecordsResponse(records)
	} else {
		loaded, err := s.Store.LoadMessages(session.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		messages = loaded
	}
	activity, err := s.Store.LoadActivity(session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	transcriptEvents, err := s.Store.LoadSessionEvents(session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	transcriptEvents = sessionevents.CompactTranscript(transcriptEvents)
	children := s.acpChildSnapshots(session.ID)
	var acpSnapshot storage.ACPState
	var hasACPSnapshot bool
	if session.Runtime == storage.RuntimeACP {
		acpSnapshot, hasACPSnapshot = s.acpSnapshot(session)
		if status := storage.SessionStatusForACPState(acpSnapshot.State); session.Status == storage.StatusRunning && status != "" {
			session.Status = status
		}
	}
	resp := map[string]any{
		"session":  canonicalSessionResponse(session),
		"messages": messages,
		"activity": activity,
		"events":   transcriptEvents,
	}
	if meta := s.acpMeta(transcriptEvents, session, children); len(meta) > 0 {
		resp["acp_meta"] = meta
	}
	if hasACPSnapshot {
		applyACPStateResponse(resp, acpSnapshot)
	}
	if len(children) > 0 {
		resp["acp_children"] = children
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSessionAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	sessionRef, action, ok := strings.Cut(rest, "/")
	if !ok || (action != "messages:stream" && action != "attachments" && action != "archive" && action != "unarchive" && action != "pin" && action != "unpin" && action != "interactive-response" && action != "permission" && action != "cancel" && action != "queue" && action != "repo/push" && action != "repo/commit" && action != "repo/merge" && action != "repo/merge-from-main" && action != "repo/restore-worktree") {
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	if sessionRef == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("session id is required"))
		return
	}
	session, err := s.Store.LoadSession(sessionRef)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	if action == "archive" || action == "unarchive" {
		if err := s.Store.SetArchived(session.ID, action == "archive"); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		session, err = s.Store.LoadSession(session.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if action == "archive" {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				s.PruneManagedWorktrees(ctx)
			}()
		}
		writeJSON(w, http.StatusOK, canonicalSessionResponse(session))
		return
	}
	if action == "pin" || action == "unpin" {
		if err := s.Store.SetPinned(session.ID, action == "pin"); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		session, err = s.Store.LoadSession(session.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, canonicalSessionResponse(session))
		return
	}
	if action == "attachments" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
			return
		}
		s.handleUploadAttachment(w, r, session)
		return
	}
	if action == "queue" {
		s.handleQueueAction(w, r, session)
		return
	}
	if action == "repo/push" {
		s.handleSessionRepoPush(w, r, session)
		return
	}
	if action == "repo/commit" {
		s.handleSessionRepoCommit(w, r, session)
		return
	}
	if action == "repo/merge" {
		s.handleSessionRepoMerge(w, r, session)
		return
	}
	if action == "repo/merge-from-main" {
		s.handleSessionRepoMergeFromMain(w, r, session)
		return
	}
	if action == "repo/restore-worktree" {
		s.handleSessionRepoRestoreWorktree(w, r, session)
		return
	}
	if action == "cancel" {
		logger := s.logger().With("session", session.ID, "runtime", session.Runtime)
		logger.Info("cancel requested", "status", session.Status)
		if session.Runtime == storage.RuntimeACP && s.ACP != nil {
			ctx, cancel := serverActionContext()
			defer cancel()
			if _, err := s.ACP.Cancel(ctx, session.ID); err != nil {
				logger.Error("acp cancel failed", "error", err)
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		} else if session.Status == storage.StatusRunning {
			// No live turn to stop (server restarted mid-turn) — unstick it.
			logger.Info("no live turn, forcing status idle")
			s.setSessionStatus(session, storage.StatusIdle)
			s.publishMessagesChanged(session.ID)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if action == "interactive-response" || action == "permission" {
		if s.ACP == nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("acp manager is not configured"))
			return
		}
		var req interactiveResponseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		ctx, cancel := serverActionContext()
		defer cancel()
		if err := s.ACP.AnswerInteractive(ctx, acp.InteractiveAnswer{
			Session:       session.ID,
			RequestID:     req.RequestID,
			OptionID:      req.OptionID,
			Text:          req.Text,
			Answers:       req.Answers,
			PlanRequested: req.PlanRequested,
			ParentVisible: req.ParentVisible,
		}); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	var req streamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("message is required"))
		return
	}
	attachments, err := s.resolveAttachments(session.ID, req.AttachmentIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	contexts := storage.NormalizeMessageContexts(append(storage.SelectionContexts(req.Quotes), req.Contexts...))

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	switch session.Runtime {
	case storage.RuntimeACP:
		s.streamACPSession(w, flusher, r.Context(), session, req.Message, contexts, attachments, req.PlanRequested)
	default:
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: fmt.Sprintf("unknown session runtime %q", session.Runtime)})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
	}
}

// Tells subscribed pages (including ones opened mid-turn) to refetch messages.
func (s *Server) publishMessagesChanged(sessionID string) {
	if s.Events == nil {
		return
	}
	s.Events.Publish(sessionevents.Event{SessionID: sessionID, Type: "assistant"})
}

func (s *Server) publishSessionChanged(sessionID string) {
	if s.Events == nil {
		return
	}
	s.Events.Publish(sessionevents.Event{SessionID: sessionID, Type: sessionevents.TypeSession})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) setSessionStatus(session storage.Session, status string) {
	s.setSessionStatusWithError(session, status, "")
}

func (s *Server) setSessionError(session storage.Session, message string) {
	s.setSessionStatusWithError(session, storage.StatusError, message)
}

func (s *Server) setSessionStatusWithError(session storage.Session, status, message string) {
	if status == "" {
		return
	}
	unlock := s.lockSession(session.ID)
	defer unlock()
	if current, err := s.Store.LoadSession(session.ID); err == nil {
		session = current
	}
	session.Status = status
	if status == storage.StatusError {
		session.Error = firstNonEmpty(message, session.Error, "Unknown error.")
	} else {
		session.Error = ""
	}
	if status == storage.StatusIdle || status == storage.StatusError {
		storage.MarkSessionAttention(&session, time.Now().UTC())
	}
	_ = s.Store.SaveSession(session)
}

func (s *Server) addUsage(sessionID string, usage *provider.Usage) {
	if usage == nil {
		return
	}
	if usageStore, ok := s.Store.(usageStore); ok {
		_ = usageStore.AddUsage(sessionID, storage.Usage{
			InputTokens:           usage.InputTokens,
			CachedInputTokens:     usage.CachedInputTokens,
			CachedWriteTokens:     usage.CachedWriteTokens,
			OutputTokens:          usage.OutputTokens,
			ReasoningOutputTokens: usage.ReasoningOutputTokens,
			TotalTokens:           usage.TotalTokens,
		})
	}
}

func titleFromMessage(message string) string {
	words := strings.Fields(message)
	if len(words) == 0 {
		return ""
	}
	if len(words) > 6 {
		words = words[:6]
	}
	title := strings.Join(words, " ")
	title = strings.Trim(title, " \t\r\n.,!?;:")
	if len(title) > 64 {
		title = strings.TrimSpace(title[:64])
	}
	return title
}

func (s *Server) lockSession(id string) func() {
	if s.Locks == nil {
		return func() {}
	}
	return s.Locks.Lock(id)
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event agent.StreamEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", event.Type)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

type streamRequest struct {
	Message       string                   `json:"message"`
	Contexts      []storage.MessageContext `json:"contexts,omitempty"`
	Quotes        []string                 `json:"quotes,omitempty"`
	AttachmentIDs []string                 `json:"attachment_ids,omitempty"`
	PlanRequested bool                     `json:"plan_requested,omitempty"`
	Voice         bool                     `json:"voice,omitempty"`
}

type interactiveResponseRequest struct {
	RequestID     string                                `json:"request_id"`
	OptionID      string                                `json:"option_id"`
	Text          string                                `json:"text,omitempty"`
	Answers       map[string]acp.InteractiveAnswerValue `json:"answers,omitempty"`
	PlanRequested bool                                  `json:"plan_requested,omitempty"`
	ParentVisible bool                                  `json:"parent_visible,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
