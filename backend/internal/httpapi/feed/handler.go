package feed

import (
	"net/http"

	feedcore "github.com/wins/jaz/backend/internal/feed"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type Handler struct {
	service feedcore.Service
}

type feedResponse struct {
	Items []feedcore.Item `json:"items"`
}

type completionResponse struct {
	Items []feedcore.Completion `json:"items"`
}

func NewHandler(service feedcore.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, _ *http.Request) {
	items, err := h.service.Feed()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, feedResponse{Items: items})
}

func (h *Handler) Completions(w http.ResponseWriter, _ *http.Request) {
	items, err := h.service.Completions()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, completionResponse{Items: items})
}
