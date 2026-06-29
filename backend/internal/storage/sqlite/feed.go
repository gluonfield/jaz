package sqlite

import (
	"context"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/feed"
)

// LoadFeed returns every unarchived thread whose newest message is an unseen
// assistant reply, newest first, each with that message attached.
func (s *Store) LoadFeed() ([]storage.FeedItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := feed.New(s.db).ListFeed(context.Background())
	if err != nil {
		return nil, err
	}
	items := make([]storage.FeedItem, 0, len(rows))
	for _, row := range rows {
		blocks, err := unmarshalBlocks(row.MessageBlocks)
		if err != nil {
			return nil, err
		}
		items = append(items, storage.FeedItem{
			ID:       row.ID,
			Slug:     row.Slug,
			Title:    row.Title.String,
			ParentID: row.ParentID.String,
			Status:   row.Status,
			LastMessage: storage.Message{
				ThreadID:  row.ID,
				Role:      row.MessageRole,
				Content:   row.MessageContent,
				Reasoning: row.MessageReasoning.String,
				Blocks:    blocks,
				CreatedAt: msToTime(row.MessageCreatedAtMs),
			},
		})
	}
	return items, nil
}
