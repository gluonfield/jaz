package feed

import (
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/threads"
)

// maxToolChars bounds the per-tool detail kept on a feed card's last message.
const maxToolChars = 280

type Service struct {
	store storage.FeedStore
}

func NewService(store storage.FeedStore) Service {
	return Service{store: store}
}

// Item is one unread thread for the Feed: identity plus its newest message in
// the compacted transcript shape the UI already renders.
type Item struct {
	ID          string
	Slug        string
	Title       string
	ParentID    string
	Status      string
	LastMessage threads.TranscriptMessage
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

func (s Service) MarkSeen(id string) error {
	return s.store.SetThreadSeen(id)
}

func lastMessage(record storage.Message) threads.TranscriptMessage {
	transcript := threads.TranscriptFromRecords([]storage.Message{record}, maxToolChars)
	if len(transcript) == 0 {
		return threads.TranscriptMessage{Role: record.Role, CreatedAt: record.CreatedAt}
	}
	return transcript[0]
}
