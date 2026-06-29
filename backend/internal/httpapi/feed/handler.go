package feed

import (
	"net/http"

	feedcore "github.com/wins/jaz/backend/internal/feed"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/threads"
)

type listHandler struct {
	service feedcore.Service
}

type feedResponse struct {
	Items []feedItemDTO `json:"items"`
}

// LastMessage is threads.TranscriptMessage, already the canonical message wire
// shape served by the transcript endpoint — no per-feature re-wrap needed.
type feedItemDTO struct {
	ID          string                    `json:"id"`
	Slug        string                    `json:"slug"`
	Title       string                    `json:"title,omitempty"`
	ParentID    string                    `json:"parent_id,omitempty"`
	Status      string                    `json:"status"`
	LastMessage threads.TranscriptMessage `json:"last_message"`
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
	httpapi.WriteJSON(w, http.StatusOK, feedResponse{Items: feedDTOs(items)})
}

func feedDTOs(items []feedcore.Item) []feedItemDTO {
	out := make([]feedItemDTO, len(items))
	for i, item := range items {
		out[i] = feedItemDTO{
			ID:          item.ID,
			Slug:        item.Slug,
			Title:       item.Title,
			ParentID:    item.ParentID,
			Status:      item.Status,
			LastMessage: item.LastMessage,
		}
	}
	return out
}
