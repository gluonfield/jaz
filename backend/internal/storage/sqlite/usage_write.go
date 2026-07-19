package sqlite

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
	usagequeries "github.com/wins/jaz/backend/internal/storage/sqlite/generated/usage"
)

func (s *Store) AddUsage(id string, usage storage.Usage) error {
	if usage.IsZero() {
		return nil
	}
	now := time.Now().UTC()
	s.writeMu.Lock()
	total := usage.TotalTokens
	if total == 0 {
		total = usage.ComponentTotal()
	}
	// Counters accumulate, while context usage snapshots the latest turn and
	// therefore may shrink after compaction.
	liveContext := usage.LiveContextTokens()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.writeMu.Unlock()
		return err
	}
	q := threaddb.New(tx)
	usageq := usagequeries.New(tx)
	thread, err := q.GetSession(ctx, id)
	if err == nil {
		nowMs := timeToMs(now)
		err = insertUsageEvent(ctx, usageq, thread, usage, total, liveContext, nowMs)
		if err == nil {
			err = q.AddUsage(ctx, addUsageParams(id, usage, total, liveContext, nowMs))
		}
	}
	if err != nil {
		_ = tx.Rollback()
		s.writeMu.Unlock()
		return err
	}
	err = tx.Commit()
	s.writeMu.Unlock()
	if err != nil {
		return err
	}
	if session, err := s.LoadSession(id); err == nil {
		s.mirrorSession(session)
	}
	return nil
}

func addUsageParams(id string, usage storage.Usage, total, liveContext, updatedAtMs int64) threaddb.AddUsageParams {
	return threaddb.AddUsageParams{
		InputTokens:           usage.InputTokens,
		CachedInputTokens:     usage.CachedInputTokens,
		CachedWriteTokens:     usage.CachedWriteTokens,
		OutputTokens:          usage.OutputTokens,
		ReasoningOutputTokens: usage.ReasoningOutputTokens,
		TotalTokens:           total,
		ContextTokens:         liveContext,
		ContextWindowTokens:   usage.ContextWindowTokens,
		UpdatedAtMs:           updatedAtMs,
		ID:                    id,
	}
}
