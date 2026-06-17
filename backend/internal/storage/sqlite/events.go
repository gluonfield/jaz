package sqlite

import (
	"context"
	"database/sql"
	stdjson "encoding/json"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) LoadSessionEvents(id string) ([]sessionevents.Event, error) {
	s.mu.Lock()
	events, err := s.loadSessionEventsLocked(id)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if len(events) > 0 || s.mirror == nil {
		return events, nil
	}
	return s.mirror.LoadSessionEvents(id)
}

func (s *Store) AppendSessionEvents(id string, events ...sessionevents.Event) error {
	if len(events) == 0 {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	defer tx.Rollback()
	eventq := eventdb.New(tx)
	threadq := threaddb.New(tx)
	nextSeq, err := eventq.NextSessionEventSeq(context.Background(), id)
	if err != nil {
		s.mu.Unlock()
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
		if err := insertSessionEvent(eventq, events[i]); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	if err := threadq.TouchThread(context.Background(), threaddb.TouchThreadParams{UpdatedAtMs: timeToMs(now), ID: id}); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := tx.Commit(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if s.mirror != nil {
		mirrored := append([]sessionevents.Event(nil), events...)
		_ = s.mirror.AppendSessionEvents(id, mirrored...)
	}
	return nil
}

func (s *Store) CompactSessionEvents(id string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	q := eventdb.New(tx)
	rows, err := q.ListSessionEvents(context.Background(), id)
	if err != nil {
		return 0, err
	}
	events := make([]sessionevents.Event, 0, len(rows))
	for _, row := range rows {
		event, err := eventFromDB(row)
		if err != nil {
			return 0, err
		}
		events = append(events, event)
	}
	runs := sessionevents.CompactTextChunkRuns(events)
	if len(runs) == 0 {
		return 0, nil
	}
	removed := 0
	for _, run := range runs {
		if err := insertSessionEvent(q, run.Event); err != nil {
			return 0, err
		}
		for _, seq := range run.DeleteSeqs {
			if err := q.DeleteSessionEvent(context.Background(), eventdb.DeleteSessionEventParams{ThreadID: id, Seq: seq}); err != nil {
				return 0, err
			}
			removed++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}

func (s *Store) loadSessionEventsLocked(id string) ([]sessionevents.Event, error) {
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

func insertSessionEvent(q *eventdb.Queries, event sessionevents.Event) error {
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
	return q.UpsertSessionEvent(context.Background(), eventdb.UpsertSessionEventParams{
		ThreadID:    event.SessionID,
		Seq:         event.Seq,
		Type:        event.Type,
		Content:     event.StorageContent(),
		Acp:         rawACP,
		Plan:        rawPlan,
		Permission:  rawPermission,
		CreatedAtMs: timeToMs(event.At),
	})
}

func eventFromDB(row eventdb.ListSessionEventsRow) (sessionevents.Event, error) {
	event := sessionevents.Event{
		SessionID: row.ThreadID,
		Seq:       row.Seq,
		Type:      row.Type,
		Content:   row.Content,
		At:        msToTime(row.CreatedAtMs),
	}
	if row.Acp.Valid && row.Acp.String != "" && row.Acp.String != "null" {
		var acp sessionevents.ACPEvent
		if err := stdjson.Unmarshal([]byte(row.Acp.String), &acp); err != nil {
			return sessionevents.Event{}, err
		}
		event.ACP = &acp
	}
	if row.Plan.Valid && row.Plan.String != "" && row.Plan.String != "null" {
		var plan sessionevents.PlanEvent
		if err := stdjson.Unmarshal([]byte(row.Plan.String), &plan); err != nil {
			return sessionevents.Event{}, err
		}
		event.Plan = &plan
	}
	if row.Permission.Valid && row.Permission.String != "" && row.Permission.String != "null" {
		var permission sessionevents.ACPPermission
		if err := stdjson.Unmarshal([]byte(row.Permission.String), &permission); err != nil {
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
