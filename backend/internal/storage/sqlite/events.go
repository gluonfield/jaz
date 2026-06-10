package sqlite

import (
	"context"
	"database/sql"
	stdjson "encoding/json"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
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
	nextSeq, err := nextSessionEventSeq(tx, id)
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
		if err := insertSessionEvent(tx, events[i]); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE threads SET updated_at_ms = ? WHERE id = ?`, timeToMs(now), id); err != nil {
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

func (s *Store) loadSessionEventsLocked(id string) ([]sessionevents.Event, error) {
	rows, err := s.db.Query(`SELECT thread_id, seq, type, content, acp, plan, permission, created_at_ms
FROM session_events WHERE thread_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []sessionevents.Event
	for rows.Next() {
		var event sessionevents.Event
		var rawACP, rawPlan, rawPermission sql.NullString
		var createdMs int64
		if err := rows.Scan(&event.SessionID, &event.Seq, &event.Type, &event.Content, &rawACP, &rawPlan, &rawPermission, &createdMs); err != nil {
			return nil, err
		}
		if rawACP.Valid && rawACP.String != "" && rawACP.String != "null" {
			var acp sessionevents.ACPEvent
			if err := stdjson.Unmarshal([]byte(rawACP.String), &acp); err != nil {
				return nil, err
			}
			event.ACP = &acp
		}
		if rawPlan.Valid && rawPlan.String != "" && rawPlan.String != "null" {
			var plan sessionevents.PlanEvent
			if err := stdjson.Unmarshal([]byte(rawPlan.String), &plan); err != nil {
				return nil, err
			}
			event.Plan = &plan
		}
		if rawPermission.Valid && rawPermission.String != "" && rawPermission.String != "null" {
			var permission sessionevents.ACPPermission
			if err := stdjson.Unmarshal([]byte(rawPermission.String), &permission); err != nil {
				return nil, err
			}
			event.Permission = &permission
		}
		event.At = msToTime(createdMs)
		events = append(events, event)
	}
	return events, rows.Err()
}

type queryer interface {
	QueryRow(string, ...any) *sql.Row
}

func nextSessionEventSeq(db queryer, threadID string) (int64, error) {
	var maxSeq sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(seq) FROM session_events WHERE thread_id = ?`, threadID).Scan(&maxSeq); err != nil {
		return 0, err
	}
	if !maxSeq.Valid {
		return 1, nil
	}
	return maxSeq.Int64 + 1, nil
}

func insertSessionEvent(db execer, event sessionevents.Event) error {
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
	_, err = db.Exec(`INSERT INTO session_events (thread_id, seq, type, content, acp, plan, permission, created_at_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(thread_id, seq) DO UPDATE SET
type = excluded.type,
content = excluded.content,
acp = excluded.acp,
plan = excluded.plan,
permission = excluded.permission,
created_at_ms = excluded.created_at_ms`,
		event.SessionID, event.Seq, event.Type, event.Content, rawACP, rawPlan, rawPermission, timeToMs(event.At))
	return err
}

func marshalOptionalJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	data, err := stdjson.Marshal(value)
	if err != nil {
		return nil, err
	}
	// Typed nils marshal to "null", which round-trips into a bogus empty struct.
	if string(data) == "null" {
		return nil, nil
	}
	return string(data), nil
}
