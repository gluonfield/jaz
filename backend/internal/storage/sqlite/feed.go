package sqlite

import (
	"context"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/feed"
)

func (s *Store) LoadFeed() ([]storage.FeedItem, error) {
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
		events, err := s.loadSessionEventsAfterTime(row.ID, promptAt)
		if err != nil {
			return nil, err
		}
		text, replyAt := lastTurnReply(events)
		if replyAt == 0 {
			replyAt = row.LastAttentionAtMs
		}
		items = append(items, storage.FeedItem{
			ID:        row.ID,
			Slug:      row.Slug,
			Title:     row.Title.String,
			ParentID:  row.ParentID.String,
			ReplyText: text,
			ReplyAt:   msToTime(replyAt),
		})
	}
	return items, nil
}

func (s *Store) LoadFeedCompletions() ([]storage.FeedCompletion, error) {
	rows, err := feed.New(s.db).ListFeedCompletions(context.Background())
	if err != nil {
		return nil, err
	}
	items := make([]storage.FeedCompletion, 0, len(rows))
	for _, row := range rows {
		items = append(items, storage.FeedCompletion{
			ID:          row.ID,
			Slug:        row.Slug,
			Title:       row.Title.String,
			CompletedAt: msToTime(row.LastCompletedAtMs),
		})
	}
	return items, nil
}

func lastTurnReply(events []sessionevents.Event) (string, int64) {
	parts := make([]string, 0)
	var replyAt int64
	for _, event := range sessionevents.CompactTextChunks(storage.DisplayEvents(events)) {
		if event.Type != sessionevents.TypeACPMessage {
			continue
		}
		if text := strings.TrimSpace(event.Content); text != "" {
			parts = append(parts, text)
		}
		replyAt = event.At.UnixMilli()
	}
	return strings.Join(parts, "\n\n"), replyAt
}
