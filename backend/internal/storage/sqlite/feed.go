package sqlite

import (
	"context"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/feed"
)

func (s *Store) LoadFeed() ([]storage.FeedItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := feed.New(s.db)
	ctx := context.Background()
	rows, err := q.ListFeed(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]storage.FeedItem, 0, len(rows))
	for _, row := range rows {
		promptAt, err := q.LastUserPromptAt(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		events, err := s.loadSessionEventsLocked(row.ID)
		if err != nil {
			return nil, err
		}
		text, replyAt := lastTurnReply(events, promptAt)
		items = append(items, storage.FeedItem{
			ID:        row.ID,
			Slug:      row.Slug,
			Title:     row.Title.String,
			ParentID:  row.ParentID.String,
			Status:    row.Status,
			ReplyText: text,
			ReplyAt:   msToTime(orDefault(replyAt, row.LastAttentionAtMs)),
		})
	}
	return items, nil
}

func lastTurnReply(events []sessionevents.Event, promptAtMs int64) (string, int64) {
	parts := make([]string, 0)
	var replyAt int64
	for _, event := range sessionevents.CompactTextChunks(events) {
		if event.Type != sessionevents.TypeACPMessage || event.At.UnixMilli() <= promptAtMs {
			continue
		}
		if text := strings.TrimSpace(event.Content); text != "" {
			parts = append(parts, text)
		}
		replyAt = event.At.UnixMilli()
	}
	return strings.Join(parts, "\n\n"), replyAt
}

func orDefault(value, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	return value
}
