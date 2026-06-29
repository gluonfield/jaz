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
		replies, err := q.LastTurnReplies(ctx, feed.LastTurnRepliesParams{
			ThreadID:  row.ID,
			ReplyType: sessionevents.TypeACPMessage,
		})
		if err != nil {
			return nil, err
		}
		parts := make([]string, 0, len(replies))
		replyAt := row.LastAttentionAtMs
		for _, reply := range replies {
			if text := strings.TrimSpace(reply.Content); text != "" {
				parts = append(parts, text)
			}
			replyAt = reply.CreatedAtMs
		}
		items = append(items, storage.FeedItem{
			ID:        row.ID,
			Slug:      row.Slug,
			Title:     row.Title.String,
			ParentID:  row.ParentID.String,
			Status:    row.Status,
			ReplyText: strings.Join(parts, "\n\n"),
			ReplyAt:   msToTime(replyAt),
		})
	}
	return items, nil
}
