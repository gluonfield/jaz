package sqlite

import (
	"context"
	"database/sql"
	stdjson "encoding/json"
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
	s.writeMu.Lock()
	err = s.appendSessionEvents(context.Background(), id, now, goalProjection.Seen, goalRaw, events)
	s.writeMu.Unlock()
	if err != nil {
		return err
	}
	if goalProjection.Seen {
		if session, loadErr := s.LoadSession(id); loadErr == nil {
			s.mirrorSession(session)
		}
	}
	return nil
}

func (s *Store) appendSessionEvents(ctx context.Context, id string, now time.Time, goalSeen bool, goalRaw string, events []sessionevents.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	eventq := eventdb.New(tx)
	threadq := threaddb.New(tx)
	nextSeq, err := eventq.NextSessionEventSeq(ctx, id)
	if err != nil {
		return err
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
		coalesceKey := sessionevents.StorageCoalesceKey(events[i])
		if coalesceKey != "" {
			if err := eventq.DeleteSessionEventByCoalesceKey(ctx, eventdb.DeleteSessionEventByCoalesceKeyParams{
				ThreadID:    events[i].SessionID,
				CoalesceKey: coalesceKey,
			}); err != nil {
				return err
			}
		}
		if err := insertSessionEvent(ctx, eventq, events[i], coalesceKey); err != nil {
			return err
		}
	}
	if goalSeen {
		err = threadq.UpdateGoal(ctx, threaddb.UpdateGoalParams{Goal: goalRaw, UpdatedAtMs: timeToMs(now), ID: id})
	} else {
		err = threadq.TouchThread(ctx, threaddb.TouchThreadParams{UpdatedAtMs: timeToMs(now), ID: id})
	}
	if err != nil {
		return err
	}
	if err := eventq.AdvanceSessionEventRevision(ctx, id); err != nil {
		return err
	}
	return tx.Commit()
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
		event, err := eventFromDBFields(row.ThreadID, row.Seq, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
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
	return q.UpsertSessionEvent(ctx, eventdb.UpsertSessionEventParams{
		ThreadID:    event.SessionID,
		Seq:         event.Seq,
		CoalesceKey: coalesceKey,
		Type:        event.Type,
		Content:     event.StorageContent(),
		Acp:         rawACP,
		Plan:        rawPlan,
		Permission:  rawPermission,
		CreatedAtMs: timeToMs(event.At),
	})
}

func eventFromDB(row eventdb.ListSessionEventsRow) (sessionevents.Event, error) {
	return eventFromDBFields(row.ThreadID, row.Seq, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
}

func eventAfterFromDB(row eventdb.ListSessionEventsAfterRow) (sessionevents.Event, error) {
	return eventFromDBFields(row.ThreadID, row.Seq, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
}

func eventFromDBFields(threadID string, seq int64, typ string, content string, acpRaw sql.NullString, planRaw sql.NullString, permissionRaw sql.NullString, createdAtMs int64) (sessionevents.Event, error) {
	event := sessionevents.Event{
		SessionID: threadID,
		Seq:       seq,
		Type:      typ,
		Content:   content,
		At:        msToTime(createdAtMs),
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
