package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
	usagequeries "github.com/wins/jaz/backend/internal/storage/sqlite/generated/usage"
)

func (s *Store) LoadACPState(id string) (storage.ACPState, error) {
	if s.mirror == nil {
		return storage.ACPState{}, fmt.Errorf("acp state store is not configured")
	}
	return s.mirror.LoadACPState(id)
}

func (s *Store) SaveACPState(id string, state storage.ACPState) error {
	if id == "" {
		return fmt.Errorf("session id is empty")
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if s.mirror == nil {
		return fmt.Errorf("acp state store is not configured")
	}
	if err := s.mirror.SaveACPState(id, state); err != nil {
		return err
	}

	status := storage.SessionStatusForACPState(state.State)
	errorMessage := ""
	if status == storage.StatusError {
		errorMessage = state.Error
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	q := threaddb.New(s.db)
	if status == "" {
		return q.TouchThread(context.Background(), threaddb.TouchThreadParams{
			UpdatedAtMs: timeToMs(state.UpdatedAt),
			ID:          id,
		})
	}
	return q.UpdateACPState(context.Background(), threaddb.UpdateACPStateParams{
		Status:      status,
		Error:       nullDBString(errorMessage),
		UpdatedAtMs: timeToMs(state.UpdatedAt),
		ID:          id,
	})
}

func (s *Store) AddUsage(id string, usage storage.Usage) error {
	if usage.IsZero() {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	total := usage.TotalTokens
	if total == 0 {
		total = usage.ComponentTotal()
	}
	// Token counters accumulate; context_tokens/context_window_tokens snapshot
	// the latest turn's live context (so it can shrink after compaction),
	// keeping the previous value when a turn reports nothing.
	liveContext := usage.LiveContextTokens()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.mu.Unlock()
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
		s.mu.Unlock()
		return err
	}
	err = tx.Commit()
	s.mu.Unlock()
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
