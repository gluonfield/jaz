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
	job, err := s.ACP.Send(ctx, acp.SendRequest{
		Session:         session.ID,
		Message:         acp.CompactCommand,
		Completion:      acp.CompletionInline,
		ActiveOperation: acp.ActiveOperationCompact,
		SkipUserMessage: true,
	})
	if err != nil {
		sendErr := acpSendError(session, err)
		s.logger().Error("acp compact failed", "session", session.ID, "error", sendErr)
		s.setSessionError(session, sendErr.Error())
		writeError(w, http.StatusBadRequest, sendErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "acp_state": job.State})
}

func acpActiveOperationForMessage(message string) string {
	if message == acp.CompactCommand {
		return acp.ActiveOperationCompact
	}
	return ""
}
