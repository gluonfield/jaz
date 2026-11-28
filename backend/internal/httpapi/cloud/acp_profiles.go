package cloud

import (
	"errors"
	"io"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
)

const WorkspaceHeader = "X-Jaz-Cloud-Workspace"

func (h *Handler) ACPProfiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var body []byte
	if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
		var err error
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			var limit *http.MaxBytesError
			status := http.StatusBadRequest
			if errors.As(err, &limit) {
				status = http.StatusRequestEntityTooLarge
			}
			httpapi.WriteError(w, status, err)
			return
		}
	}
	raw, status, err := h.service.ACPProfiles(r.Context(), r.Header.Get(WorkspaceHeader), r.Method, r.URL.Path, body)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}
