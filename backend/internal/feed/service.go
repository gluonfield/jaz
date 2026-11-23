package feed

import (
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/threads"
)

type Service struct {
	store     storage.FeedStore
	workspace func() string
}

func NewService(store storage.FeedStore) Service {
	return Service{store: store}
}

func (s Service) WithWorkspace(fn func() string) Service {
	s.workspace = fn
	return s
}

func (s Service) activeWorkspaceID() string {
	if s.workspace != nil {
		if ws := s.workspace(); ws != "" {
			return ws
		}
	}
	return storage.LocalWorkspaceID
}

type Item struct {
	ID          string                    `json:"id"`
	Slug        string                    `json:"slug"`
	Title       string                    `json:"title,omitempty"`
	ParentID    string                    `json:"parent_id,omitempty"`
	LastMessage threads.TranscriptMessage `json:"last_message"`
}

func (s Service) Feed() ([]Item, error) {
	var (
		entries []storage.FeedItem
		err     error
	)
	if wsStore, ok := s.store.(storage.WorkspaceFeedStore); ok {
		entries, err = wsStore.LoadFeedForWorkspace(s.activeWorkspaceID())
	} else {
		entries, err = s.store.LoadFeed()
	}
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
