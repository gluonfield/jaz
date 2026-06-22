package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

type sideChatRequest struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

func (s *Server) handleSessionSideChat(w http.ResponseWriter, r *http.Request) {
	sessionRef := r.PathValue("session")
	if sessionRef == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("session id is required"))
		return
	}
	session, err := s.Store.LoadSession(sessionRef)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil || acp.CanonicalAgentName(session.RuntimeRef.Agent) != acp.AgentCodex || s.ACP == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("side chat requires a codex acp session"))
		return
	}
	var req sideChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := serverSideChatContext()
	defer cancel()
	if err := s.ACP.SendSideChat(ctx, acp.SideChatRequest{
		Session: session.ID,
		ID:      req.ID,
		Message: req.Message,
	}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
