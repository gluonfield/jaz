package server

import (
	"fmt"
	"net/http"

	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	"github.com/wins/jaz/backend/internal/sessionview"
	"github.com/wins/jaz/backend/internal/storage"
)

func knownSessionAction(action string) bool {
	switch action {
	case "messages:stream",
		"attachments",
		"archive",
		"unarchive",
		"pin",
		"unpin",
		"seen",
		"rename",
		"interactive-response",
		"permission",
		"cancel",
		"compact",
		"queue",
		"repo/push",
		"repo/commit",
		"repo/merge",
		"repo/merge-branches",
		"repo/merge-from-main",
		"repo/restore-worktree":
		return true
	default:
		return false
	}
}

func (s *Server) handleCloudSessionAction(w http.ResponseWriter, r *http.Request, session storage.Session, action string) {
	var patch storage.CloudThreadUserStatePatch
	switch action {
	case "archive", "unarchive":
		v := action == "archive"
		patch.Archived = &v
	case "pin", "unpin":
		v := action == "pin"
		patch.Pinned = &v
	case "seen":
		v := false
		patch.Unread = &v
	case "cancel":
		raw, status, err := s.Cloud.CancelThread(r.Context(), session.ID)
		if err != nil {
			cloudapi.WriteError(w, err)
			return
		}
		cloudapi.WriteRawJSON(w, status, raw)
		return
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("action %q is not supported for cloud threads", action))
		return
	}
	updated, _, err := s.Cloud.UpdateUserState(r.Context(), session.ID, patch)
	if err != nil {
		cloudapi.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionview.Public(updated))
}
