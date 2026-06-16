package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
	usagequeries "github.com/wins/jaz/backend/internal/storage/sqlite/generated/usage"
)

func (s *Store) UsageEventsSince(since time.Time) ([]storage.UsageEvent, error) {
	s.mu.Lock()
	rows, err := usagequeries.New(s.db).ListUsageEventsSince(context.Background(), timeToMs(since.In(time.UTC)))
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	events := make([]storage.UsageEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, storage.UsageEvent{
			SessionID:     row.ThreadID,
			Runtime:       row.Runtime,
			Agent:         row.Agent,
			ModelProvider: row.ModelProvider,
			Model:         row.Model,
			Usage: storage.Usage{
				InputTokens:           row.InputTokens,
				CachedInputTokens:     row.CachedInputTokens,
				CachedWriteTokens:     row.CachedWriteTokens,
				OutputTokens:          row.OutputTokens,
				ReasoningOutputTokens: row.ReasoningOutputTokens,
				TotalTokens:           row.TotalTokens,
			},
			CreatedAt: msToTime(row.CreatedAtMs),
		})
	}
	return events, nil
}

func insertUsageEvent(ctx context.Context, q usagequeries.Querier, thread threaddb.Thread, usage storage.Usage, total, liveContext, createdAtMs int64) error {
	eventUsage := usage
	eventUsage.TotalTokens = total
	eventUsage.ContextTokens = liveContext
	if !eventUsage.Countable() {
		return nil
	}
	return q.InsertUsageEvent(ctx, usagequeries.InsertUsageEventParams{
		ThreadID:              thread.ID,
		Runtime:               thread.Runtime,
		Agent:                 nullString(thread.AcpAgent),
		ModelProvider:         nullString(thread.ModelProvider),
		Model:                 nullString(thread.Model),
		InputTokens:           usage.InputTokens,
		CachedInputTokens:     usage.CachedInputTokens,
		CachedWriteTokens:     usage.CachedWriteTokens,
		OutputTokens:          usage.OutputTokens,
		ReasoningOutputTokens: usage.ReasoningOutputTokens,
		TotalTokens:           total,
		ContextTokens:         liveContext,
		ContextWindowTokens:   usage.ContextWindowTokens,
		CreatedAtMs:           createdAtMs,
	})
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
