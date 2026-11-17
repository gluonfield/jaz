package plugins

import (
	"encoding/json"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/plugins"
)

type ViewHandler struct {
	service *plugins.ViewService
}

func NewViewHandler(service *plugins.ViewService) *ViewHandler {
	return &ViewHandler{service: service}
}

func (h *ViewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
