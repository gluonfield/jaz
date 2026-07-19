package jsonstore

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Store) LoadSessionEvents(id string) ([]sessionevents.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSessionEvents(id)
}

func (s *Store) LoadSessionEventsAfter(id string, afterSeq int64) ([]sessionevents.Event, error) {
	if afterSeq <= 0 {
		return s.LoadSessionEvents(id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := s.loadSessionEvents(id)
	if err != nil {
		return nil, err
	}
	filtered := events[:0]
	for _, event := range events {
		if event.Seq > afterSeq {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func (s *Store) LoadLatestACPTurn(ctx context.Context, id string) ([]sessionevents.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := s.loadSessionEvents(id)
	if err != nil {
		return nil, err
	}
	return sessionevents.LatestACPTurn(events, id), nil
}

func (s *Store) loadSessionEvents(id string) ([]sessionevents.Event, error) {
	path := filepath.Join(s.sessionDir(id), "events.jsonl")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return unmarshalSessionEventsJSONL(data)
}

func (s *Store) AppendSessionEvents(id string, events ...sessionevents.Event) error {
	if len(events) == 0 {
		return nil
	}
	expanded, last := sessionevents.SplitTextEvents(events, storage.MaxTextEventBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.EnsureSession(id); err != nil {
		return err
	}
	needsHistory := false
	for _, event := range expanded {
		if event.Seq == 0 || sessionevents.NeedsProviderSubagentSnapshot(event) {
			needsHistory = true
			break
		}
	}
	var existing []sessionevents.Event
	if needsHistory {
		var err error
		existing, err = s.loadSessionEvents(id)
		if err != nil {
			return err
		}
	}
	nextSeq := int64(1)
	for _, event := range existing {
		nextSeq = max(nextSeq, event.Seq+1)
	}
	now := time.Now().UTC()
	for i := range expanded {
		if expanded[i].SessionID == "" {
			expanded[i].SessionID = id
		}
		if expanded[i].At.IsZero() {
			expanded[i].At = now
		}
		if expanded[i].Seq == 0 {
			expanded[i].Seq = nextSeq
			nextSeq++
		}
		expanded[i] = sessionevents.EnsureStatelessProjection(expanded[i])
		if sessionevents.NeedsProviderSubagentSnapshot(expanded[i]) {
			for j := len(existing) - 1; j >= 0; j-- {
				if existing[j].ProjectionKey == expanded[i].ProjectionKey {
					expanded[i] = sessionevents.CompleteProviderSubagentSnapshot(existing[j], expanded[i])
					break
				}
			}
		}
		data, err := stdjson.Marshal(expanded[i])
		if err != nil {
			return err
		}
		if len(data) > storage.MaxSessionEventBytes {
			return fmt.Errorf("session event is %d bytes; limit is %d", len(data), storage.MaxSessionEventBytes)
		}
		existing = append(existing, expanded[i])
	}
	goalProjection, err := storage.GoalProjectionFromEvents(expanded...)
	if err != nil {
		return err
	}
	path := filepath.Join(s.sessionDir(id), "events.jsonl")
	if err := appendSessionEventsJSONL(path, expanded); err != nil {
		return err
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
	if goalProjection.Seen {
		session, err := s.loadSessionByID(id)
		if err != nil {
			return err
		}
		session.Goal = goalProjection.State
		return s.saveSession(session)
	}
	s.touchSession(id)
	return nil
}

func unmarshalSessionEventsJSONL(data []byte) ([]sessionevents.Event, error) {
	dec := stdjson.NewDecoder(bytes.NewReader(data))
	var events []sessionevents.Event
	for {
		var event sessionevents.Event
		if err := dec.Decode(&event); err != nil {
			if err == io.EOF {
				return events, nil
			}
			return nil, err
		}
		event.NormalizePayload()
		events = append(events, event)
	}
}

func appendSessionEventsJSONL(path string, events []sessionevents.Event) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := stdjson.NewEncoder(file)
	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			return err
		}
	}
	return nil
}
