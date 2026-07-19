package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

const (
	eventCompactionBatchOperations = 8
	eventCompactionBatchBytes      = 4 << 20
	eventCompactionPageRows        = 8
	eventCompactionTextBytes       = storage.MaxTextEventBytes
)

type SessionEventCompaction struct {
	ThreadID string
	Removed  int
	Complete bool
}

type sessionEventDelete struct {
	seq   int64
	bytes int
}

type sessionEventMutation struct {
	seq           int64
	key           string
	projectionKey string
	projectionOp  string
	snapshot      *sessionevents.Event
	snapshotBytes int
	latestSize    int
	deletes       []sessionEventDelete
}

type sessionEventCompactionPlan struct {
	id            string
	revision      int64
	afterSeq      int64
	annotator     sessionevents.Annotator
	mutations     []sessionEventMutation
	next          int
	pageAfterSeq  int64
	pageAnnotator sessionevents.Annotator
	pageEOF       bool
}

type eventCompactionPlanner struct {
	mu   sync.Mutex
	plan *sessionEventCompactionPlan
}

func (s *Store) SessionEventCompactionWake() <-chan struct{} {
	return s.compactionWake
}

func (s *Store) notifySessionEventCompaction() {
	select {
	case s.compactionWake <- struct{}{}:
	default:
	}
}

func (s *Store) CompactNextSessionEvents(ctx context.Context) (SessionEventCompaction, error) {
	s.eventPlanner.mu.Lock()
	defer s.eventPlanner.mu.Unlock()
	for {
		if s.eventPlanner.plan == nil {
			plan, err := s.loadSessionEventCompactionPlan(ctx)
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
		if s.eventPlanner.plan.mutations == nil {
			if err := s.prepareSessionEventCompactionPage(ctx, s.eventPlanner.plan); err != nil {
				id := s.eventPlanner.plan.id
				s.eventPlanner.plan = nil
				return SessionEventCompaction{ThreadID: id}, err
			}
		}
		result, current, err := s.applySessionEventCompactionPlan(ctx, s.eventPlanner.plan)
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

func (s *Store) HasPendingSessionEventCompaction(ctx context.Context) (bool, error) {
	pending, err := eventdb.New(s.db).HasPendingSessionEventCompaction(ctx, storage.StatusRunning)
	return pending, err
}

func (s *Store) loadSessionEventCompactionPlan(ctx context.Context) (*sessionEventCompactionPlan, error) {
	q := eventdb.New(s.db)
	id, err := q.NextSessionEventCompaction(ctx, storage.StatusRunning)
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
	return &sessionEventCompactionPlan{id: id, revision: state.EventRevision}, nil
}

func (s *Store) prepareSessionEventCompactionPage(ctx context.Context, plan *sessionEventCompactionPlan) error {
	rows, err := eventdb.New(s.db).ListSessionEventCompactionPage(ctx, eventdb.ListSessionEventCompactionPageParams{
		ThreadID:   plan.id,
		AfterSeq:   plan.afterSeq,
		LimitCount: eventCompactionPageRows,
	})
	if err != nil {
		return err
	}
	events := make([]sessionevents.Event, 0, len(rows))
	sizeBySeq := make(map[int64]int, len(rows))
	coalesceKeyBySeq := make(map[int64]string, len(rows))
	projectionKeyBySeq := make(map[int64]string, len(rows))
	projectionOpBySeq := make(map[int64]string, len(rows))
	for _, row := range rows {
		rowBytes := sessionEventFieldSize(row.Content, row.Acp, row.Plan, row.Permission)
		if rowBytes > storage.MaxSessionEventBytes {
			sizeErr := fmt.Errorf("session event %d is %d bytes; limit is %d", row.Seq, rowBytes, storage.MaxSessionEventBytes)
			if skipErr := s.skipSessionEventCompaction(ctx, plan.id, plan.revision); skipErr != nil {
				return errors.Join(sizeErr, fmt.Errorf("mark session event compaction skipped: %w", skipErr))
			}
			return sizeErr
		}
		event, err := eventFromDBFields(row.ThreadID, row.Seq, row.ProjectionKey, row.ProjectionOp, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
		if err != nil {
			decodeErr := fmt.Errorf("decode session events for compaction: %w", err)
			if skipErr := s.skipSessionEventCompaction(ctx, plan.id, plan.revision); skipErr != nil {
				return errors.Join(decodeErr, fmt.Errorf("mark session event compaction skipped: %w", skipErr))
			}
			return decodeErr
		}
		events = append(events, event)
		sizeBySeq[event.Seq] = rowBytes
		coalesceKeyBySeq[event.Seq] = row.CoalesceKey
		projectionKeyBySeq[event.Seq] = row.ProjectionKey
		projectionOpBySeq[event.Seq] = row.ProjectionOp
	}
	annotator := plan.annotator.Clone()
	for i := range events {
		events[i] = annotator.Annotate(events[i])
	}
	byKey := make(map[string]int)
	for _, event := range events {
		if sessionevents.NeedsStorageCompaction(event) {
			continue
		}
		slim := event.SlimForStorage()
		key := sessionevents.StorageCoalesceKey(slim)
		if key == "" {
			if event.NeedsStorageSlimming() || event.ProjectionKey != projectionKeyBySeq[event.Seq] || event.ProjectionOp != projectionOpBySeq[event.Seq] {
				snapshot := slim
				bytes, sizeErr := sessionEventStorageSize(snapshot)
				if sizeErr != nil {
					return sizeErr
				}
				plan.mutations = append(plan.mutations, sessionEventMutation{
					seq: event.Seq, key: coalesceKeyBySeq[event.Seq], snapshot: &snapshot,
					snapshotBytes: bytes, latestSize: sizeBySeq[event.Seq],
				})
			}
			continue
		}
		if index, ok := byKey[key]; ok {
			mutation := &plan.mutations[index]
			mutation.deletes = append(mutation.deletes, sessionEventDelete{seq: mutation.seq, bytes: mutation.latestSize})
			mutation.seq = event.Seq
			snapshot := slim
			mutation.snapshot = &snapshot
			mutation.snapshotBytes, err = sessionEventStorageSize(snapshot)
			if err != nil {
				return err
			}
			mutation.latestSize = sizeBySeq[event.Seq]
			continue
		}
		byKey[key] = len(plan.mutations)
		snapshot := slim
		bytes, sizeErr := sessionEventStorageSize(snapshot)
		if sizeErr != nil {
			return sizeErr
		}
		plan.mutations = append(plan.mutations, sessionEventMutation{
			seq:           event.Seq,
			key:           key,
			snapshot:      &snapshot,
			snapshotBytes: bytes,
			latestSize:    sizeBySeq[event.Seq],
		})
	}
	for _, segment := range sessionevents.CompactTextSegments(events, eventCompactionTextBytes) {
		if segment.Event.ProjectionKey == "" {
			continue
		}
		key := segment.Event.ProjectionKey + ":segment:" + strconv.FormatInt(segment.Event.Seq, 10)
		deletes := make([]sessionEventDelete, 0, len(segment.DeleteSeqs))
		for _, seq := range segment.DeleteSeqs {
			deletes = append(deletes, sessionEventDelete{seq: seq, bytes: sizeBySeq[seq]})
		}
		storedKey := coalesceKeyBySeq[segment.Event.Seq]
		if len(deletes) == 0 && storedKey == key && projectionKeyBySeq[segment.Event.Seq] == segment.Event.ProjectionKey && projectionOpBySeq[segment.Event.Seq] == segment.Event.ProjectionOp {
			continue
		}
		mutation := sessionEventMutation{
			seq: segment.Event.Seq, key: key, projectionKey: segment.Event.ProjectionKey,
			projectionOp: segment.Event.ProjectionOp, latestSize: sizeBySeq[segment.Event.Seq], deletes: deletes,
		}
		if len(deletes) > 0 || sizeBySeq[segment.Event.Seq] <= eventCompactionTextBytes {
			snapshot := segment.Event.SlimForStorage()
			mutation.snapshot = &snapshot
			mutation.snapshotBytes, err = sessionEventStorageSize(snapshot)
			if err != nil {
				return err
			}
		}
		plan.mutations = append(plan.mutations, mutation)
	}
	plan.pageAfterSeq = plan.afterSeq
	if len(rows) > 0 {
		plan.pageAfterSeq = rows[len(rows)-1].Seq
	}
	plan.pageAnnotator = annotator
	plan.pageEOF = len(rows) < eventCompactionPageRows
	return nil
}

func (s *Store) applySessionEventCompactionPlan(ctx context.Context, plan *sessionEventCompactionPlan) (SessionEventCompaction, bool, error) {
	if !s.writeMu.tryMaintenanceLock() {
		return SessionEventCompaction{ThreadID: plan.id}, true, nil
	}
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

	next := plan.next
	removed := 0
	operations := 0
	bytes := 0
	for next < len(plan.mutations) && operations < eventCompactionBatchOperations {
		mutation := &plan.mutations[next]
		previousBytes := 0
		removePrevious := false
		if mutation.snapshot != nil && mutation.key != "" {
			previous, previousErr := q.GetSessionEventByCoalesceKey(ctx, eventdb.GetSessionEventByCoalesceKeyParams{
				ThreadID: plan.id, CoalesceKey: mutation.key,
			})
			if previousErr == nil {
				previousBytes = sessionEventFieldSize(previous.Content, previous.Acp, previous.Plan, previous.Permission)
				removePrevious = previous.Seq != mutation.seq
			} else if !errors.Is(previousErr, sql.ErrNoRows) {
				return SessionEventCompaction{ThreadID: plan.id}, true, previousErr
			}
		}
		writeBytes := mutation.snapshotBytes + mutation.latestSize + previousBytes
		if mutation.snapshot == nil {
			writeBytes = mutation.latestSize
		}
		seqs := make([]int64, len(mutation.deletes))
		for i, item := range mutation.deletes {
			seqs[i] = item.seq
			writeBytes += item.bytes
		}
		if writeBytes > eventCompactionBatchBytes {
			skipped, skipErr := q.SkipSessionEventCompaction(ctx, eventdb.SkipSessionEventCompactionParams{
				ThreadID: plan.id, EventRevision: plan.revision, RunningStatus: storage.StatusRunning,
			})
			if skipErr != nil {
				return SessionEventCompaction{ThreadID: plan.id}, true, skipErr
			}
			if skipped == 0 {
				return SessionEventCompaction{ThreadID: plan.id}, false, nil
			}
			if err := tx.Commit(); err != nil {
				return SessionEventCompaction{ThreadID: plan.id}, true, err
			}
			return SessionEventCompaction{ThreadID: plan.id, Complete: true}, true, nil
		}
		if bytes+writeBytes > eventCompactionBatchBytes {
			break
		}
		if mutation.snapshot != nil && mutation.key != "" {
			_, err = q.DeleteSessionEventByCoalesceKey(ctx, eventdb.DeleteSessionEventByCoalesceKeyParams{
				ThreadID: plan.id, CoalesceKey: mutation.key,
			})
		}
		if err == nil && len(seqs) > 0 {
			err = q.DeleteSessionEvents(ctx, eventdb.DeleteSessionEventsParams{ThreadID: plan.id, Seqs: seqs})
		}
		if err == nil && mutation.snapshot != nil {
			err = insertSessionEvent(ctx, q, *mutation.snapshot, mutation.key)
		} else if err == nil {
			err = q.SetSessionEventProjection(ctx, eventdb.SetSessionEventProjectionParams{
				CoalesceKey:   mutation.key,
				ProjectionKey: mutation.projectionKey,
				ProjectionOp:  mutation.projectionOp,
				ThreadID:      plan.id,
				Seq:           mutation.seq,
			})
		}
		if err != nil {
			return SessionEventCompaction{ThreadID: plan.id}, true, err
		}
		operations++
		bytes += writeBytes
		removed += len(seqs)
		if removePrevious {
			removed++
		}
		next++
	}
	complete := false
	pageComplete := next == len(plan.mutations)
	if pageComplete && plan.pageEOF {
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
	if operations > 0 {
		if err := threaddb.New(tx).AdvanceTranscriptRevision(ctx, plan.id); err != nil {
			return SessionEventCompaction{ThreadID: plan.id}, true, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SessionEventCompaction{ThreadID: plan.id}, true, err
	}
	if pageComplete {
		plan.afterSeq = plan.pageAfterSeq
		plan.annotator = plan.pageAnnotator
		plan.mutations = nil
		plan.next = 0
		plan.pageAfterSeq = 0
		plan.pageAnnotator = sessionevents.Annotator{}
		plan.pageEOF = false
	} else {
		plan.next = next
	}
	return SessionEventCompaction{ThreadID: plan.id, Removed: removed, Complete: complete}, true, nil
}

func (s *Store) skipSessionEventCompaction(ctx context.Context, id string, revision int64) error {
	if !s.writeMu.tryMaintenanceLock() {
		return nil
	}
	defer s.writeMu.Unlock()
	_, err := eventdb.New(s.db).SkipSessionEventCompaction(ctx, eventdb.SkipSessionEventCompactionParams{
		ThreadID:      id,
		EventRevision: revision,
		RunningStatus: storage.StatusRunning,
	})
	return err
}

func sessionEventFieldSize(content string, acp, plan, permission sql.NullString) int {
	size := len(content)
	if acp.Valid {
		size += len(acp.String)
	}
	if plan.Valid {
		size += len(plan.String)
	}
	if permission.Valid {
		size += len(permission.String)
	}
	return size
}

func sessionEventStorageSize(event sessionevents.Event) (int, error) {
	acp, err := marshalOptionalJSON(event.ACP)
	if err != nil {
		return 0, err
	}
	plan, err := marshalOptionalJSON(event.Plan)
	if err != nil {
		return 0, err
	}
	permission, err := marshalOptionalJSON(event.Permission)
	if err != nil {
		return 0, err
	}
	return sessionEventFieldSize(event.StorageContent(), acp, plan, permission), nil
}
