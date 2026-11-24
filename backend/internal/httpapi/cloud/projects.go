package cloud

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
)

type ProjectsHandler struct {
	service *cloudsync.Service
}

func NewProjectsHandler(service *cloudsync.Service) *ProjectsHandler {
	return &ProjectsHandler{service: service}
}

func (h *ProjectsHandler) GetSpace(w http.ResponseWriter, r *http.Request) {
	raw, status, err := h.service.GetSpace(r.Context(), r.PathValue("id"))
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *ProjectsHandler) DeleteSpace(w http.ResponseWriter, r *http.Request) {
	raw, status, err := h.service.DeleteSpace(r.Context(), r.PathValue("id"))
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *ProjectsHandler) ProxySpaceSourceContent(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.OpenSpaceSourceContent(r.Context(), r.PathValue("id"), r.PathValue("source_id"))
	if err != nil {
		WriteError(w, err)
		return
	}
	defer resp.Body.Close()
	for _, header := range []string{"Content-Type", "Content-Disposition", "Content-Length", "ETag"} {
		if v := resp.Header.Get(header); v != "" {
			w.Header().Set(header, v)
		}
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *ProjectsHandler) PatchThread(w http.ResponseWriter, r *http.Request) {
	body, err := ReadLimitedBody(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	raw, status, err := h.service.UpdateThread(r.Context(), r.PathValue("id"), body)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *ProjectsHandler) DeleteThread(w http.ResponseWriter, r *http.Request) {
	raw, status, err := h.service.DeleteThread(r.Context(), r.PathValue("id"))
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *ProjectsHandler) UpdateThreadUserState(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Pinned                 *bool  `json:"pinned"`
		Archived               *bool  `json:"archived"`
		PersonalSourcesEnabled *bool  `json:"personal_sources_enabled"`
		Unread                 *bool  `json:"unread"`
		LastReadSeq            *int64 `json:"last_read_seq"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	unread := body.Unread
	if unread == nil && body.LastReadSeq != nil && *body.LastReadSeq > 0 {
		v := false
		unread = &v
	}
	_, raw, err := h.service.UpdateUserState(r.Context(), r.PathValue("id"), storage.CloudThreadUserStatePatch{
		Pinned:                 body.Pinned,
		Archived:               body.Archived,
		PersonalSourcesEnabled: body.PersonalSourcesEnabled,
		Unread:                 unread,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, http.StatusOK, raw)
}
