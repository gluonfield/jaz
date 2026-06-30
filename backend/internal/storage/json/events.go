package jsonstore

import (
	"bytes"
	stdjson "encoding/json"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.EnsureSession(id); err != nil {
		return err
	}
	existing, err := s.loadSessionEvents(id)
	if err != nil {
		return err
	}
	nextSeq := int64(len(existing) + 1)
	now := time.Now().UTC()
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
	}
	goalProjection, err := storage.GoalProjectionFromEvents(events...)
	if err != nil {
		return err
	}
	path := filepath.Join(s.sessionDir(id), "events.jsonl")
	if err := appendSessionEventsJSONL(path, events); err != nil {
		return err
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
