package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
)

type ACPManager interface {
	Send(context.Context, acp.SendRequest) (acp.Job, error)
	Status(string) (acp.Job, error)
}

type Server struct {
	Agent        *agent.Agent
	Store        storage.Store
	ACP          ACPManager
	Locks        *sessionlock.Locks
	Events       *sessionevents.Bus
	SystemPrompt string
	Root         string
}

type messageRecordStore interface {
	LoadMessageRecords(string) ([]storage.Message, error)
}

type reasoningMessageStore interface {
	SaveMessagesWithReasoning(string, []provider.Message, map[int]string) error
}

type usageStore interface {
	AddUsage(string, storage.Usage) error
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	mux.HandleFunc("GET /v1/sessions/", s.handleGetSession)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("POST /v1/sessions/", s.handleSessionAction)
	mux.HandleFunc("GET /v1/agent/files", s.handleListAgentFiles)
	mux.HandleFunc("PUT /v1/agent/files/{name}", s.handleWriteAgentFile)
	return withCORS(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	session, err := s.Store.CreateSession(storage.CreateSession{
		Slug:    req.Slug,
		Title:   req.Title,
		Runtime: storage.RuntimeNative,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("last") == "true" {
		session, err := s.Store.LastRootSession()
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, session)
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
		Archived:        query.Get("archived") == "true",
		Limit:           limit,
	}
	sessions, err := s.Store.ListSessions(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	sessionRef, action, hasAction := strings.Cut(rest, "/")
	if sessionRef == "" || (hasAction && action != "messages" && action != "events") {
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	session, err := s.Store.LoadSession(sessionRef)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if !hasAction {
		writeJSON(w, http.StatusOK, session)
		return
	}
	if action == "events" {
		s.streamSessionEvents(w, r, session.ID)
		return
	}
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
	resp := map[string]any{"session": session, "messages": messages, "activity": activity}
	if session.Runtime == storage.RuntimeACP && s.ACP != nil {
		if job, err := s.ACP.Status(session.ID); err == nil {
			resp["acp_state"] = job.State
		}
	}
	writeJSON(w, http.StatusOK, resp)
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
	if !ok || (action != "messages:stream" && action != "archive" && action != "unarchive") {
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
		session.Archived = action == "archive"
		writeJSON(w, http.StatusOK, session)
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
		s.streamNativeSession(w, flusher, r, session, req.Message)
	case storage.RuntimeACP:
		s.streamACPSession(w, flusher, r.Context(), session, req.Message)
	default:
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: fmt.Sprintf("unknown session runtime %q", session.Runtime)})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
	}
}

func (s *Server) streamNativeSession(w http.ResponseWriter, flusher http.Flusher, r *http.Request, session storage.Session, message string) {
	unlock := s.lockSession(session.ID)
	defer unlock()

	session.Status = storage.StatusRunning
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeNative
	}
	if session.Title == "" {
		session.Title = titleFromMessage(message)
	}
	_ = s.Store.SaveSession(session)

	messages, err := s.Store.LoadMessages(session.ID)
	if err != nil {
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		s.setSessionStatus(session, storage.StatusError)
		return
	}
	if len(messages) == 0 && s.SystemPrompt != "" {
		messages = append(messages, provider.SystemMessage(s.SystemPrompt))
	}
	messages = append(messages, provider.UserMessage(message))

	ctx := sessioncontext.WithSessionID(r.Context(), session.ID)
	finalStatus := storage.StatusError
	for event := range s.Agent.Run(ctx, provider.Request{Messages: messages}) {
		if len(event.Messages) > 0 {
			var err error
			if store, ok := s.Store.(reasoningMessageStore); ok && len(event.ReasoningByMessage) > 0 {
				err = store.SaveMessagesWithReasoning(session.ID, event.Messages, event.ReasoningByMessage)
			} else {
				err = s.Store.SaveMessages(session.ID, event.Messages)
			}
			if err != nil {
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
				s.setSessionStatus(session, storage.StatusError)
				return
			}
		}
		if event.Type == agent.StreamDone {
			finalStatus = storage.StatusIdle
			s.addUsage(session.ID, event.Usage)
		}
		if event.Type == agent.StreamError {
			finalStatus = storage.StatusError
		}
		writeSSE(w, flusher, event)
	}
	s.setSessionStatus(session, finalStatus)
}

func (s *Server) streamACPSession(w http.ResponseWriter, flusher http.Flusher, ctx context.Context, session storage.Session, message string) {
	if s.ACP == nil {
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: "acp manager is not configured"})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
		return
	}
	unlock := s.lockSession(session.ID)
	defer unlock()

	s.setSessionStatus(session, storage.StatusRunning)
	job, err := s.ACP.Send(ctx, acp.SendRequest{Session: session.ID, Message: message, Completion: acp.CompletionInline})
	if err != nil {
		s.setSessionStatus(session, storage.StatusError)
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: acpSendError(session, err).Error()})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
		return
	}

	emittedAssistant := 0
	emittedThought := 0
	seenTools := map[string]struct{}{}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	for {
		emitACPJob(w, flusher, job, &emittedAssistant, &emittedThought, seenTools)
		if job.State == acp.StateFailed {
			s.setSessionStatus(session, storage.StatusError)
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: job.Error})
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
			return
		}
		if isACPTerminal(job.State) {
			s.setSessionStatus(session, storage.StatusIdle)
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, err = s.ACP.Status(session.ID)
			if err != nil {
				s.setSessionStatus(session, storage.StatusError)
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
				return
			}
		}
	}
}

func emitACPJob(w http.ResponseWriter, flusher http.Flusher, job acp.Job, emittedAssistant, emittedThought *int, seenTools map[string]struct{}) {
	for _, call := range job.ToolCalls {
		key := firstNonEmpty(call.ID, call.Title)
		if key == "" {
			continue
		}
		if _, ok := seenTools[key]; ok {
			continue
		}
		seenTools[key] = struct{}{}
		writeSSE(w, flusher, agent.StreamEvent{
			Type:     agent.StreamToolCall,
			ToolName: firstNonEmpty(call.Title, call.ID),
		})
	}
	if *emittedAssistant < len(job.Assistant) {
		delta := job.Assistant[*emittedAssistant:]
		*emittedAssistant = len(job.Assistant)
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDelta, Delta: delta})
	}
	if *emittedThought < len(job.Thought) {
		delta := job.Thought[*emittedThought:]
		*emittedThought = len(job.Thought)
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamReasoning, Reasoning: delta})
	}
}

func isACPTerminal(state string) bool {
	return state == acp.StateIdle || state == acp.StateCancelled
}

func acpSendError(session storage.Session, err error) error {
	if strings.Contains(err.Error(), "active acp session not found") {
		return fmt.Errorf("acp session %q (%s) is stored but not active in this server process; reconnect/resume is not implemented yet", session.Slug, session.ID)
	}
	return err
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
	if status == "" {
		return
	}
	if current, err := s.Store.LoadSession(session.ID); err == nil {
		session = current
	}
	session.Status = status
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
	Message string `json:"message"`
}

type createSessionRequest struct {
	Slug  string `json:"slug,omitempty"`
	Title string `json:"title,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
