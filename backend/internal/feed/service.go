package feed

import (
	"time"

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
	LastMessage threads.TranscriptMessage `json:"last_message"`
}

type Completion struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title,omitempty"`
	CompletedAt time.Time `json:"completed_at"`
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
			LastMessage: threads.TranscriptMessage{
				Role:      "assistant",
				Text:      entry.ReplyText,
				CreatedAt: entry.ReplyAt,
			},
		})
	}
	return items, nil
}

func (s Service) Completions() ([]Completion, error) {
	entries, err := s.store.LoadFeedCompletions()
	if err != nil {
		return nil, err
	}
	items := make([]Completion, 0, len(entries))
	for _, entry := range entries {
		items = append(items, Completion{
			ID:          entry.ID,
			Slug:        entry.Slug,
			Title:       entry.Title,
			CompletedAt: entry.CompletedAt,
		})
	}
	return items, nil
}
