package sqlite

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
	usagequeries "github.com/wins/jaz/backend/internal/storage/sqlite/generated/usage"
)

var _ storage.WorkspaceUsageEventStore = (*Store)(nil)

func (s *Store) UsageEvents(since, until time.Time) ([]storage.UsageEvent, error) {
	workspaceID, err := s.LoadActiveWorkspaceID(context.Background())
	if err != nil {
		workspaceID = storage.LocalWorkspaceID
	}
	return s.UsageEventsForWorkspace(workspaceID, since, until)
}

func (s *Store) UsageEventsForWorkspace(workspaceID string, since, until time.Time) ([]storage.UsageEvent, error) {
	ws := strings.TrimSpace(workspaceID)
	if ws == "" {
		ws = storage.LocalWorkspaceID
	}
	end := int64(math.MaxInt64)
	if !until.IsZero() {
		end = ceilUnixMilli(until)
	}
	rows, err := usagequeries.New(s.db).ListUsageEvents(context.Background(), usagequeries.ListUsageEventsParams{
		Since:       ceilUnixMilli(since),
		Until:       end,
		WorkspaceID: ws,
	})
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
			Source:     row.Source,
			SourceType: row.SourceType,
			CreatedAt:  msToTime(row.CreatedAtMs),
		})
	}
	return events, nil
}

func ceilUnixMilli(t time.Time) int64 {
	ms := t.UnixMilli()
	if t.Nanosecond()%1_000_000 != 0 {
		ms++
	}
	return ms
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
		WorkspaceID:           firstNonEmpty(thread.WorkspaceID, storage.LocalWorkspaceID),
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
		Source:                storage.UsageEventSourceTurn,
		SourceType:            nullString(thread.SourceType),
		CreatedAtMs:           createdAtMs,
	})
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
