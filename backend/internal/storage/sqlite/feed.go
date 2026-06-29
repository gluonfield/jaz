package sqlite

import (
	"context"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/feed"
)

func (s *Store) LoadFeed() ([]storage.FeedItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := feed.New(s.db).ListFeed(context.Background(), sessionevents.TypeACPMessage)
	if err != nil {
		return nil, err
	}
	items := make([]storage.FeedItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, storage.FeedItem{
			ID:        row.ID,
			Slug:      row.Slug,
			Title:     row.Title.String,
			ParentID:  row.ParentID.String,
			Status:    row.Status,
			ReplyText: row.MessageContent.String,
			ReplyAt:   msToTime(row.MessageCreatedAtMs),
		})
	}
	return items, nil
}
