package feed

import (
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/threads"
)

type Service struct {
	store storage.FeedStore
}

func NewService(store storage.FeedStore) Service {
	return Service{store: store}
}

type Item struct {
	ID          string                    `json:"id"`
	Slug        string                    `json:"slug"`
	Title       string                    `json:"title,omitempty"`
	ParentID    string                    `json:"parent_id,omitempty"`
	Status      string                    `json:"status"`
	LastMessage threads.TranscriptMessage `json:"last_message"`
}

func (s Service) Feed() ([]Item, error) {
	entries, err := s.store.LoadFeed()
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, Item{
			ID:       entry.ID,
			Slug:     entry.Slug,
			Title:    entry.Title,
			ParentID: entry.ParentID,
			Status:   entry.Status,
			LastMessage: threads.TranscriptMessage{
				Role:      "assistant",
				Text:      strings.TrimSpace(entry.ReplyText),
				CreatedAt: entry.ReplyAt,
			},
		})
	}
	return items, nil
}
