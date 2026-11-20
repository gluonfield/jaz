package plugins

import (
	"encoding/json"
	"io"
	"net/http"

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
	state, err := h.service.State(r.Context(), r.PathValue("session"), r.URL.Query().Get("reference"), r.URL.Query().Get("file"))
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, state)
}

func (h *ViewHandler) SaveState(w http.ResponseWriter, r *http.Request) {
	state, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxViewStateBytes))
	if err != nil {
		httpapi.WriteError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	if err := h.service.SaveState(r.Context(), r.PathValue("session"), r.URL.Query().Get("reference"), r.URL.Query().Get("file"), state); err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
