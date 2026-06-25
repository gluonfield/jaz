package server

import (
	"fmt"
	"net/http"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Server) handleSessionCompact(w http.ResponseWriter, r *http.Request, session storage.Session) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	if session.Runtime != storage.RuntimeACP {
		writeError(w, http.StatusBadRequest, fmt.Errorf("compact is only available for acp sessions"))
		return
	}
	if !sessionSupportsCompact(session) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("compact is not available for this session"))
		return
	}
	if s.ACP == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("acp manager is not configured"))
		return
	}
	session, err := s.beginACPTurn(r.Context(), session, "Compact")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := serverActionContextFrom(r.Context())
	defer cancel()
	job, err := s.ACP.Compact(ctx, acp.CompactRequest{Session: session.ID})
	if err != nil {
		sendErr := acpSendError(session, err)
		s.logger().Error("acp compact failed", "session", session.ID, "error", sendErr)
		s.setSessionError(session, sendErr.Error())
		writeError(w, http.StatusBadRequest, sendErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "acp_state": job.State})
}

func isACPCompactCommand(message string) bool {
	return message == acp.CompactCommand
}
