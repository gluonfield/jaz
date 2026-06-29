package feed

import (
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/threads"
)

const maxToolChars = 280

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
			ID:          entry.ID,
			Slug:        entry.Slug,
			Title:       entry.Title,
			ParentID:    entry.ParentID,
			Status:      entry.Status,
			LastMessage: lastMessage(entry.LastMessage),
		})
	}
	return items, nil
}

func lastMessage(record storage.Message) threads.TranscriptMessage {
	transcript := threads.TranscriptFromRecords([]storage.Message{record}, maxToolChars)
	if len(transcript) == 0 {
		return threads.TranscriptMessage{Role: record.Role, CreatedAt: record.CreatedAt}
	}
	return transcript[0]
}
