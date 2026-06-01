package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/storage"
)

type Server struct {
	Agent        *agent.Agent
	Store        storage.Store
	SystemPrompt string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("POST /v1/sessions/", s.handleSessionAction)
	return mux
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
		Limit:           limit,
	}
	sessions, err := s.Store.ListSessions(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) handleSessionAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	sessionRef, action, ok := strings.Cut(rest, "/")
	if !ok || action != "messages:stream" {
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

	messages, err := s.Store.LoadMessages(session.ID)
	if err != nil {
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		return
	}
	if len(messages) == 0 && s.SystemPrompt != "" {
		messages = append(messages, provider.SystemMessage(s.SystemPrompt))
	}
	messages = append(messages, provider.UserMessage(req.Message))

	ctx := sessioncontext.WithSessionID(r.Context(), session.ID)
	for event := range s.Agent.Run(ctx, provider.Request{Messages: messages}) {
		if len(event.Messages) > 0 {
			_ = s.Store.SaveMessages(session.ID, event.Messages)
		}
		writeSSE(w, flusher, event)
	}
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
