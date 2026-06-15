package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/coordinator"
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
	"github.com/wins/jaz/backend/internal/voice"
	"github.com/wins/jaz/backend/internal/widgets"
)

type ACPManager interface {
	CreateSession(context.Context, acp.SpawnRequest) (storage.Session, error)
	Spawn(context.Context, acp.SpawnRequest) (acp.SpawnResult, error)
	Send(context.Context, acp.SendRequest) (acp.Job, error)
	Status(string) (acp.Job, error)
	List() []acp.Job
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
	Agent           *agent.Agent
	Store           storage.Store
	ACP             ACPManager
	MCP             MCPRuntime
	Locks           *sessionlock.Locks
	Events          *sessionevents.Bus
	Loops           *loops.Service
	Widgets         *widgets.Service
	STT             voice.STT
	TTS             voice.TTS
	NativeProviders provider.ReloadableProvider
	AgentCatalog    acp.AgentCatalog
	AuthKey         string
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
	terminalOnce sync.Once

	// in-flight native turns by session id, cancellable via the cancel action
	turnCancels      sync.Map
	acpAuthLoginJobs sync.Map
	worktreePruneMu  sync.Mutex
}

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
	catalog, err := skills.Load(s.Root)
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

func canonicalSessionResponses(sessions []storage.Session) []storage.Session {
	out := make([]storage.Session, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, canonicalSessionResponse(session))
	}
	return out
}

func canonicalSessionResponse(session storage.Session) storage.Session {
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return session
	}
	ref := *session.RuntimeRef
	canonical := acp.CanonicalAgentName(ref.Agent)
	if canonical == "" {
		session.RuntimeRef = &ref
		return session
	}
	if session.ModelProvider != "" && acp.CanonicalAgentName(session.ModelProvider) == canonical {
		session.ModelProvider = canonical
	}
	ref.Agent = canonical
	session.RuntimeRef = &ref
	return session
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
		messages = records
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
	resp := map[string]any{
		"session":  canonicalSessionResponse(session),
		"messages": messages,
		"activity": activity,
		"events":   transcriptEvents,
	}
	if meta := s.acpMeta(transcriptEvents, session, children); len(meta) > 0 {
		resp["acp_meta"] = meta
	}
	if session.Runtime == storage.RuntimeACP {
		if state, ok := s.acpSnapshot(session); ok {
			applyACPStateResponse(resp, state)
		}
	}
	if len(children) > 0 {
		resp["acp_children"] = children
	}
	writeJSON(w, http.StatusOK, resp)
}

type acpMetaEntry struct {
	Title string `json:"title,omitempty"`
	Slug  string `json:"slug,omitempty"`
}

// Stored events carry only the acp session id and slug; the titles the UI
// labels them with live here, once per response instead of per row. The page
// session and child snapshots are already in hand, so only ids outside that
// set hit the store.
func (s *Server) acpMeta(events []sessionevents.Event, session storage.Session, children []storage.ACPState) map[string]acpMetaEntry {
	ids := map[string]bool{}
	for _, event := range events {
		if event.ACP != nil && event.ACP.ID != "" {
			ids[event.ACP.ID] = true
		}
	}
	childByID := make(map[string]storage.ACPState, len(children))
	for _, child := range children {
		childByID[child.ID] = child
	}
	meta := make(map[string]acpMetaEntry, len(ids))
	for id := range ids {
		if id == session.ID {
			meta[id] = acpMetaEntry{Title: session.Title, Slug: session.Slug}
			continue
		}
		if child, ok := childByID[id]; ok {
			meta[id] = acpMetaEntry{Title: child.Title, Slug: child.Slug}
			continue
		}
		if ref, err := s.Store.LoadSession(id); err == nil {
			meta[id] = acpMetaEntry{Title: ref.Title, Slug: ref.Slug}
		}
	}
	return meta
}

func (s *Server) acpSnapshot(session storage.Session) (storage.ACPState, bool) {
	if s.ACP != nil {
		if job, err := s.ACP.Status(session.ID); err == nil && job.State != "not_running" {
			return canonicalACPStateResponse(acpJobState(job)), true
		}
	}
	if state, err := s.Store.LoadACPState(session.ID); err == nil {
		return canonicalACPStateResponse(state), true
	}
	if s.ACP != nil {
		if job, err := s.ACP.Status(session.ID); err == nil {
			return canonicalACPStateResponse(acpJobState(job)), true
		}
	}
	if session.Runtime == storage.RuntimeACP {
		return canonicalACPStateResponse(acpStateFromSession(session)), true
	}
	return storage.ACPState{}, false
}

func (s *Server) acpChildSnapshots(parentID string) []storage.ACPState {
	byID := map[string]storage.ACPState{}
	children, err := s.Store.ListSessions(storage.SessionFilter{
		ParentID:   parentID,
		ParentOnly: true,
		Runtime:    storage.RuntimeACP,
	})
	if err == nil {
		for _, child := range children {
			if state, ok := s.acpSnapshot(child); ok {
				if !state.ParentVisible {
					continue
				}
				byID[state.ID] = state
			}
		}
	}
	if s.ACP != nil {
		for _, job := range s.ACP.List() {
			if job.ParentID == parentID && job.ParentVisible {
				byID[job.ID] = canonicalACPStateResponse(acpJobState(job))
			}
		}
	}
	out := make([]storage.ACPState, 0, len(byID))
	for _, state := range byID {
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func applyACPStateResponse(resp map[string]any, state storage.ACPState) {
	state = canonicalACPStateResponse(state)
	resp["acp_state"] = state.State
	resp["acp_assistant"] = state.Assistant
	resp["acp_thought"] = state.Thought
	resp["acp_modes"] = state.Modes
	resp["acp_plan"] = state.Plan
	resp["acp_tool_calls"] = state.ToolCalls
	resp["acp_permissions"] = state.Permissions
	resp["acp_error"] = state.Error
}

func canonicalACPStateResponse(state storage.ACPState) storage.ACPState {
	if canonical := acp.CanonicalAgentName(state.ACPAgent); canonical != "" {
		state.ACPAgent = canonical
	}
	return state
}

func acpJobState(job acp.Job) storage.ACPState {
	plan := make([]sessionevents.ACPPlanEntry, 0, len(job.Plan))
	for _, entry := range job.Plan {
		plan = append(plan, sessionevents.ACPPlanEntry{
			Content:  entry.Content,
			Status:   entry.Status,
			Priority: entry.Priority,
		})
	}
	calls := make([]sessionevents.ACPToolCall, 0, len(job.ToolCalls))
	for _, call := range job.ToolCalls {
		calls = append(calls, sessionevents.ACPToolCall{
			ID:     call.ID,
			Title:  call.Title,
			Status: call.Status,
		})
	}
	return storage.ACPState{
		ID:          job.ID,
		Slug:        job.Slug,
		Title:       job.Title,
		ParentID:    job.ParentID,
		ACPAgent:    acp.CanonicalAgentName(job.ACPAgent),
		ACPSession:  job.ACPSession,
		Cwd:         job.Cwd,
		State:       job.State,
		StopReason:  job.StopReason,
		Assistant:   job.Assistant,
		Thought:     job.Thought,
		Plan:        plan,
		ToolCalls:   calls,
		Permissions: job.Permissions,
		Modes: sessionevents.ACPModeState{
			CurrentModeID:   job.Modes.CurrentModeID,
			ExecutionModeID: job.Modes.ExecutionModeID,
			PlanModeID:      job.Modes.PlanModeID,
			AvailableModes:  acpModes(job.Modes.AvailableModes),
		},
		Error:         job.Error,
		ParentVisible: job.ParentVisible,
		CreatedAt:     job.CreatedAt,
		UpdatedAt:     job.UpdatedAt,
	}
}

func acpModes(in []acp.ModeSnapshot) []sessionevents.ACPMode {
	out := make([]sessionevents.ACPMode, 0, len(in))
	for _, mode := range in {
		out = append(out, sessionevents.ACPMode{
			ID:          mode.ID,
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	return out
}

func acpStateFromSession(session storage.Session) storage.ACPState {
	session = canonicalSessionResponse(session)
	state := storage.ACPState{
		ID:        session.ID,
		Slug:      session.Slug,
		Title:     session.Title,
		ParentID:  session.ParentID,
		State:     "not_running",
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
	}
	if session.RuntimeRef != nil {
		state.ACPAgent = session.RuntimeRef.Agent
		state.ACPSession = session.RuntimeRef.SessionID
		state.Cwd = session.RuntimeRef.Cwd
	}
	return state
}

func (s *Server) streamSessionEvents(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.Events == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("session events are not configured"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	// Flush the headers immediately so EventSource clients see the stream
	// open before the first event arrives.
	flusher.Flush()
	for event := range s.Events.Subscribe(r.Context(), sessionID) {
		writeSessionEventSSE(w, flusher, event)
	}
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
		} else if cancel, ok := s.turnCancels.Load(session.ID); ok {
			cancel.(context.CancelFunc)()
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
	case "", storage.RuntimeNative:
		s.streamNativeSession(w, flusher, r, session, req.Message, attachments, req.Voice, req.PlanRequested)
	case storage.RuntimeACP:
		s.streamACPSession(w, flusher, r.Context(), session, req.Message, attachments, req.PlanRequested)
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

func writeSessionEventSSE(w http.ResponseWriter, flusher http.Flusher, event sessionevents.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", event.Type)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

type streamRequest struct {
	Message       string   `json:"message"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
	PlanRequested bool     `json:"plan_requested,omitempty"`
	Voice         bool     `json:"voice,omitempty"`
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
