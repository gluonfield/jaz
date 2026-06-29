package feed

import (
	"net/http"

	feedcore "github.com/wins/jaz/backend/internal/feed"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type listHandler struct {
	service feedcore.Service
}

type feedResponse struct {
	Items []feedcore.Item `json:"items"`
}

func NewListHandler(service feedcore.Service) http.Handler {
	return listHandler{service: service}
}

func (h listHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Feed()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, feedResponse{Items: items})
}
