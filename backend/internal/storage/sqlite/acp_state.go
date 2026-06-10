package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
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
	s.mu.Lock()
	total := usage.TotalTokens
	if total == 0 {
		total = usage.ComponentTotal()
	}
	// Token counters accumulate; context_tokens/context_window_tokens snapshot
	// the latest turn's live context (so it can shrink after compaction),
	// keeping the previous value when a turn reports nothing.
	liveContext := usage.LiveContextTokens()
	err := threaddb.New(s.db).AddUsage(context.Background(), threaddb.AddUsageParams{
		InputTokens:           usage.InputTokens,
		CachedInputTokens:     usage.CachedInputTokens,
		CachedWriteTokens:     usage.CachedWriteTokens,
		OutputTokens:          usage.OutputTokens,
		ReasoningOutputTokens: usage.ReasoningOutputTokens,
		TotalTokens:           total,
		ContextTokens:         liveContext,
		ContextWindowTokens:   usage.ContextWindowTokens,
		UpdatedAtMs:           timeToMs(time.Now().UTC()),
		ID:                    id,
	})
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if session, err := s.LoadSession(id); err == nil {
		s.mirrorSession(session)
	}
	return nil
}
