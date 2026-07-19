package sqlite

import (
	"context"
	"database/sql"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) LoadSessionEvents(id string) ([]sessionevents.Event, error) {
	return s.loadSessionEvents(id)
}

func (s *Store) LoadSessionEventsAfter(id string, afterSeq int64) ([]sessionevents.Event, error) {
	if afterSeq <= 0 {
		return s.LoadSessionEvents(id)
	}
	return s.loadSessionEventsAfter(id, afterSeq)
}

func (s *Store) LoadLatestACPTurn(ctx context.Context, id string) ([]sessionevents.Event, error) {
	rows, err := eventdb.New(s.db).ListLatestACPTurn(ctx, id)
	if err != nil {
		return nil, err
	}
	events := make([]sessionevents.Event, 0, len(rows))
	for _, row := range rows {
		event, decodeErr := eventFromDBFields(row.ThreadID, row.Seq, row.ProjectionKey, row.ProjectionOp, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
		if decodeErr != nil {
			return nil, decodeErr
		}
		events = append(events, event)
	}
	return sessionevents.CompactTranscript(events), nil
}

func (s *Store) AppendSessionEvents(id string, events ...sessionevents.Event) error {
	if len(events) == 0 {
		return nil
	}
	now := time.Now().UTC()
	goalProjection, err := storage.GoalProjectionFromEvents(events...)
	if err != nil {
		return err
	}
	goalRaw := ""
	if goalProjection.Seen {
		goalRaw, err = storage.MarshalGoalState(goalProjection.State)
		if err != nil {
			return err
		}
	}
	expanded, last := sessionevents.SplitTextEvents(events, storage.MaxTextEventBytes)
	s.writeMu.Lock()
	compactionPending, appendErr := s.appendSessionEvents(context.Background(), id, now, goalProjection.Seen, goalRaw, expanded)
	s.writeMu.Unlock()
	if appendErr != nil {
		return appendErr
	}
	for i := range events {
		stored := expanded[last[i]]
		events[i].Seq = stored.Seq
		events[i].At = stored.At
		events[i].SessionID = stored.SessionID
		events[i].ProjectionKey = stored.ProjectionKey
		events[i].ProjectionOp = stored.ProjectionOp
		if stored.Type == sessionevents.TypeProviderSubagent {
			events[i].ProviderSubagent = stored.ProviderSubagent
			events[i].Content = stored.Content
		}
	}
	if compactionPending {
		s.notifySessionEventCompaction()
	}
	if goalProjection.Seen {
		if session, loadErr := s.LoadSession(id); loadErr == nil {
			s.mirrorSession(session)
		}
	}
	return nil
}

func (s *Store) appendSessionEvents(ctx context.Context, id string, now time.Time, goalSeen bool, goalRaw string, events []sessionevents.Event) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	eventq := eventdb.New(tx)
	threadq := threaddb.New(tx)
	var compactionPending int64
	historyChanged := false
	nextSeq, err := eventq.NextSessionEventSeq(ctx, id)
	if err != nil {
		return false, err
	}
	// Mutate in place so callers see the assigned Seq/At after the append.
	for i := range events {
		if events[i].SessionID == "" {
			events[i].SessionID = id
		}
		if events[i].At.IsZero() {
			events[i].At = now
		}
		if events[i].Seq == 0 {
			events[i].Seq = nextSeq
			nextSeq++
		}
		events[i] = sessionevents.EnsureStatelessProjection(events[i])
		if sessionevents.NeedsStorageCompaction(events[i]) {
			compactionPending = 1
		}
		coalesceKey := sessionevents.StorageCoalesceKey(events[i])
		if coalesceKey != "" {
			if sessionevents.NeedsProviderSubagentSnapshot(events[i]) {
				previous, loadErr := eventq.GetSessionEventByCoalesceKey(ctx, eventdb.GetSessionEventByCoalesceKeyParams{
					ThreadID:    events[i].SessionID,
					CoalesceKey: coalesceKey,
				})
				if loadErr == nil {
					stored, decodeErr := eventFromDBFields(previous.ThreadID, previous.Seq, previous.ProjectionKey, previous.ProjectionOp, previous.Type, previous.Content, previous.Acp, previous.Plan, previous.Permission, previous.CreatedAtMs)
					if decodeErr != nil {
						return false, decodeErr
					}
					events[i] = sessionevents.CompleteProviderSubagentSnapshot(stored, events[i])
				} else if !errors.Is(loadErr, sql.ErrNoRows) {
					return false, loadErr
				}
			}
			deleted, err := eventq.DeleteSessionEventByCoalesceKey(ctx, eventdb.DeleteSessionEventByCoalesceKeyParams{
				ThreadID:    events[i].SessionID,
				CoalesceKey: coalesceKey,
			})
			if err != nil {
				return false, err
			}
			historyChanged = historyChanged || deleted > 0
		}
		if err := insertSessionEvent(ctx, eventq, events[i], coalesceKey); err != nil {
			return false, err
		}
	}
	if goalSeen {
		err = threadq.UpdateGoal(ctx, threaddb.UpdateGoalParams{Goal: goalRaw, UpdatedAtMs: timeToMs(now), ID: id})
	} else {
		err = threadq.TouchThread(ctx, threaddb.TouchThreadParams{UpdatedAtMs: timeToMs(now), ID: id})
	}
	if err != nil {
		return false, err
	}
	if err := eventq.AdvanceSessionEventRevision(ctx, eventdb.AdvanceSessionEventRevisionParams{
		CompactionPending: compactionPending,
		ThreadID:          id,
	}); err != nil {
		return false, err
	}
	if historyChanged {
		if err := threadq.AdvanceTranscriptRevision(ctx, id); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return compactionPending != 0, nil
}

func (s *Store) loadSessionEvents(id string) ([]sessionevents.Event, error) {
	rows, err := eventdb.New(s.db).ListSessionEvents(context.Background(), id)
	if err != nil {
		return nil, err
	}
	events := make([]sessionevents.Event, 0, len(rows))
	for _, row := range rows {
		event, err := eventFromDB(row)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) loadSessionEventsAfterTime(id string, afterMs int64) ([]sessionevents.Event, error) {
	rows, err := eventdb.New(s.db).ListSessionEventsAfterTime(context.Background(), eventdb.ListSessionEventsAfterTimeParams{
		ThreadID: id,
		AfterMs:  afterMs,
	})
	if err != nil {
		return nil, err
	}
	events := make([]sessionevents.Event, 0, len(rows))
	for _, row := range rows {
		event, err := eventFromDBFields(row.ThreadID, row.Seq, row.ProjectionKey, row.ProjectionOp, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) loadSessionEventsAfter(id string, afterSeq int64) ([]sessionevents.Event, error) {
	rows, err := eventdb.New(s.db).ListSessionEventsAfter(context.Background(), eventdb.ListSessionEventsAfterParams{
		ThreadID: id,
		AfterSeq: afterSeq,
	})
	if err != nil {
		return nil, err
	}
	events := make([]sessionevents.Event, 0, len(rows))
	for _, row := range rows {
		event, err := eventAfterFromDB(row)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func insertSessionEvent(ctx context.Context, q *eventdb.Queries, event sessionevents.Event, coalesceKey string) error {
	rawACP, err := marshalOptionalJSON(event.ACP)
	if err != nil {
		return err
	}
	rawPermission, err := marshalOptionalJSON(event.Permission)
	if err != nil {
		return err
	}
	rawPlan, err := marshalOptionalJSON(event.Plan)
	if err != nil {
		return err
	}
	if size := sessionEventFieldSize(event.StorageContent(), rawACP, rawPlan, rawPermission); size > storage.MaxSessionEventBytes {
		return fmt.Errorf("session event is %d bytes; limit is %d", size, storage.MaxSessionEventBytes)
	}
	return q.UpsertSessionEvent(ctx, eventdb.UpsertSessionEventParams{
		ThreadID:      event.SessionID,
		Seq:           event.Seq,
		CoalesceKey:   coalesceKey,
		ProjectionKey: event.ProjectionKey,
		ProjectionOp:  event.ProjectionOp,
		Type:          event.Type,
		Content:       event.StorageContent(),
		Acp:           rawACP,
		Plan:          rawPlan,
		Permission:    rawPermission,
		CreatedAtMs:   timeToMs(event.At),
	})
}

func eventFromDB(row eventdb.ListSessionEventsRow) (sessionevents.Event, error) {
	return eventFromDBFields(row.ThreadID, row.Seq, row.ProjectionKey, row.ProjectionOp, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
}

func eventAfterFromDB(row eventdb.ListSessionEventsAfterRow) (sessionevents.Event, error) {
	return eventFromDBFields(row.ThreadID, row.Seq, row.ProjectionKey, row.ProjectionOp, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
}

func eventFromDBFields(threadID string, seq int64, projectionKey, projectionOp, typ string, content string, acpRaw sql.NullString, planRaw sql.NullString, permissionRaw sql.NullString, createdAtMs int64) (sessionevents.Event, error) {
	event := sessionevents.Event{
		SessionID:     threadID,
		Seq:           seq,
		ProjectionKey: projectionKey,
		ProjectionOp:  projectionOp,
		Type:          typ,
		Content:       content,
		At:            msToTime(createdAtMs),
	}
	if acpRaw.Valid && acpRaw.String != "" && acpRaw.String != "null" {
		var acp sessionevents.ACPEvent
		if err := stdjson.Unmarshal([]byte(acpRaw.String), &acp); err != nil {
			return sessionevents.Event{}, err
		}
		event.ACP = &acp
	}
	if planRaw.Valid && planRaw.String != "" && planRaw.String != "null" {
		var plan sessionevents.PlanEvent
		if err := stdjson.Unmarshal([]byte(planRaw.String), &plan); err != nil {
			return sessionevents.Event{}, err
		}
		event.Plan = &plan
	}
	if permissionRaw.Valid && permissionRaw.String != "" && permissionRaw.String != "null" {
		var permission sessionevents.ACPPermission
		if err := stdjson.Unmarshal([]byte(permissionRaw.String), &permission); err != nil {
			return sessionevents.Event{}, err
		}
		event.Permission = &permission
	}
	event.NormalizePayload()
	return event, nil
}

func marshalOptionalJSON(value any) (sql.NullString, error) {
	if value == nil {
		return sql.NullString{}, nil
	}
	data, err := stdjson.Marshal(value)
	if err != nil {
		return sql.NullString{}, err
	}
	// Typed nils marshal to "null", which round-trips into a bogus empty struct.
	if string(data) == "null" {
		return sql.NullString{}, nil
	}
	return sql.NullString{String: string(data), Valid: true}, nil
}
