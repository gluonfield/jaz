package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
)

const (
	legacyCompactionBatchOperations = 64
	legacyCompactionBatchBytes      = 4 << 20
)

type SessionEventCompaction struct {
	ThreadID string
	Removed  int
	Complete bool
}

type legacyEventDelete struct {
	seq   int64
	bytes int
}

type legacyEventMutation struct {
	seq        int64
	key        string
	prepared   bool
	latestSize int
	deletes    []legacyEventDelete
	deleted    int
}

type legacyEventPlan struct {
	id        string
	revision  int64
	mutations []legacyEventMutation
	next      int
}

type legacyEventPlanner struct {
	mu   sync.Mutex
	plan *legacyEventPlan
}

func (s *Store) CompactNextLegacySessionEvents(ctx context.Context) (SessionEventCompaction, error) {
	s.eventPlanner.mu.Lock()
	defer s.eventPlanner.mu.Unlock()
	for {
		if s.eventPlanner.plan == nil {
			plan, err := s.loadLegacyEventPlan(ctx)
			if err != nil {
				result := SessionEventCompaction{}
				if plan != nil {
					result.ThreadID = plan.id
				}
				return result, err
			}
			if plan == nil {
				return SessionEventCompaction{}, nil
			}
			s.eventPlanner.plan = plan
		}
		result, current, err := s.applyLegacyEventPlan(ctx, s.eventPlanner.plan)
		if err != nil {
			return result, err
		}
		if current {
			if result.Complete {
				s.eventPlanner.plan = nil
			}
			return result, nil
		}
		s.eventPlanner.plan = nil
	}
}

func (s *Store) HasLegacySessionEventThreads(ctx context.Context) (bool, error) {
	pending, err := eventdb.New(s.db).HasLegacySessionEventThreads(ctx)
	return pending, err
}

func (s *Store) loadLegacyEventPlan(ctx context.Context) (*legacyEventPlan, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := eventdb.New(tx)
	id, err := q.NextLegacySessionEventThread(ctx, storage.StatusRunning)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state, err := q.GetSessionEventCompactionState(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSessionEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	plan := &legacyEventPlan{id: id, revision: state.EventRevision}
	byKey := make(map[string]int)
	for _, row := range rows {
		event, err := eventFromDB(row)
		if err != nil {
			decodeErr := fmt.Errorf("decode legacy session events: %w", err)
			if skipErr := s.skipLegacySessionEvents(ctx, id, state.EventRevision); skipErr != nil {
				return plan, errors.Join(decodeErr, fmt.Errorf("mark legacy session events skipped: %w", skipErr))
			}
			return plan, decodeErr
		}
		size := legacyEventRowSize(row)
		key := sessionevents.StorageCoalesceKey(event)
		if key == "" {
			continue
		}
		if index, ok := byKey[key]; ok {
			mutation := &plan.mutations[index]
			mutation.deletes = append(mutation.deletes, legacyEventDelete{seq: mutation.seq, bytes: mutation.latestSize})
			mutation.seq = event.Seq
			mutation.latestSize = size
			continue
		}
		byKey[key] = len(plan.mutations)
		plan.mutations = append(plan.mutations, legacyEventMutation{
			seq:        event.Seq,
			key:        key,
			latestSize: size,
		})
	}
	return plan, nil
}

func (s *Store) applyLegacyEventPlan(ctx context.Context, plan *legacyEventPlan) (SessionEventCompaction, bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionEventCompaction{ThreadID: plan.id}, true, err
	}
	defer tx.Rollback()
	q := eventdb.New(tx)
	state, err := q.GetSessionEventCompactionState(ctx, plan.id)
	if err != nil {
		return SessionEventCompaction{ThreadID: plan.id}, true, err
	}
	if state.EventCompactionVersion != 0 || state.EventRevision != plan.revision || state.Status == storage.StatusRunning {
		return SessionEventCompaction{ThreadID: plan.id}, false, nil
	}

	type mutationProgress struct {
		index    int
		prepared bool
		deleted  int
	}
	progress := make([]mutationProgress, 0, legacyCompactionBatchOperations)
	next := plan.next
	removed := 0
	operations := 0
	bytes := 0
	for next < len(plan.mutations) && operations < legacyCompactionBatchOperations {
		mutation := &plan.mutations[next]
		prepared := mutation.prepared
		deleted := mutation.deleted
		if !prepared {
			err = q.SetSessionEventCoalesceKey(ctx, eventdb.SetSessionEventCoalesceKeyParams{
				CoalesceKey: mutation.key,
				ThreadID:    plan.id,
				Seq:         mutation.seq,
			})
			if err != nil {
				return SessionEventCompaction{ThreadID: plan.id}, true, err
			}
			prepared = true
			operations++
		}
		for deleted < len(mutation.deletes) && operations < legacyCompactionBatchOperations {
			item := mutation.deletes[deleted]
			if bytes > 0 && bytes+item.bytes > legacyCompactionBatchBytes {
				break
			}
			if err := q.DeleteSessionEvent(ctx, eventdb.DeleteSessionEventParams{ThreadID: plan.id, Seq: item.seq}); err != nil {
				return SessionEventCompaction{ThreadID: plan.id}, true, err
			}
			deleted++
			operations++
			bytes += item.bytes
			removed++
		}
		progress = append(progress, mutationProgress{index: next, prepared: prepared, deleted: deleted})
		if deleted < len(mutation.deletes) {
			break
		}
		next++
	}
	complete := false
	if next == len(plan.mutations) {
		completed, err := q.CompleteSessionEventCompaction(ctx, eventdb.CompleteSessionEventCompactionParams{
			ThreadID:      plan.id,
			EventRevision: plan.revision,
			RunningStatus: storage.StatusRunning,
		})
		if err != nil {
			return SessionEventCompaction{ThreadID: plan.id}, true, err
		}
		if completed == 0 {
			return SessionEventCompaction{ThreadID: plan.id}, false, nil
		}
		complete = true
	}
	if err := tx.Commit(); err != nil {
		return SessionEventCompaction{ThreadID: plan.id}, true, err
	}
	for _, update := range progress {
		plan.mutations[update.index].prepared = update.prepared
		plan.mutations[update.index].deleted = update.deleted
	}
	plan.next = next
	return SessionEventCompaction{ThreadID: plan.id, Removed: removed, Complete: complete}, true, nil
}

func (s *Store) skipLegacySessionEvents(ctx context.Context, id string, revision int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := eventdb.New(s.db).SkipSessionEventCompaction(ctx, eventdb.SkipSessionEventCompactionParams{
		ThreadID:      id,
		EventRevision: revision,
		RunningStatus: storage.StatusRunning,
	})
	return err
}

func legacyEventRowSize(row eventdb.ListSessionEventsRow) int {
	size := len(row.Content)
	if row.Acp.Valid {
		size += len(row.Acp.String)
	}
	if row.Plan.Valid {
		size += len(row.Plan.String)
	}
	if row.Permission.Valid {
		size += len(row.Permission.String)
	}
	return size
}
