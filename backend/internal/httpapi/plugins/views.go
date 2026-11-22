package plugins

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/plugins"
)

const maxViewStateBytes = 200_000

type ViewHandler struct {
	service *plugins.ViewService
}

func NewViewHandler(service *plugins.ViewService) *ViewHandler {
	return &ViewHandler{service: service}
}

func (h *ViewHandler) Open(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reference string `json:"reference"`
		File      string `json:"file"`
		Name      string `json:"name"`
		Title     string `json:"title"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	view, err := h.service.Open(r.Context(), r.PathValue("session"), plugins.ViewRequest{Reference: input.Reference, File: input.File, Name: input.Name, Title: input.Title})
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, view)
}

func (h *ViewHandler) State(w http.ResponseWriter, r *http.Request) {
	state, rev, err := h.service.StateWithRevision(r.Context(), r.PathValue("session"), r.URL.Query().Get("reference"), r.URL.Query().Get("file"))
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	if rev > 0 {
		w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(rev, 10)))
	}
	httpapi.WriteJSON(w, http.StatusOK, state)
}

func (h *ViewHandler) SaveState(w http.ResponseWriter, r *http.Request) {
	state, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxViewStateBytes))
	if err != nil {
		httpapi.WriteError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	var expectedRev *int64
	if raw := strings.Trim(strings.TrimSpace(r.Header.Get("If-Match")), `"`); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			httpapi.WriteError(w, http.StatusBadRequest, parseErr)
			return
		}
		expectedRev = &parsed
	}
	rev, err := h.service.SaveStateWithRevision(r.Context(), r.PathValue("session"), r.URL.Query().Get("reference"), r.URL.Query().Get("file"), state, expectedRev)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	if rev > 0 {
		w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(rev, 10)))
	}
	w.WriteHeader(http.StatusNoContent)
}
