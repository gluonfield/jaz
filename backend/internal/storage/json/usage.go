package jsonstore

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Store) UsageEventsSince(since time.Time) ([]storage.UsageEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events, hasEvents, err := s.loadUsageEventsSince(since)
	if err != nil {
		return nil, err
	}
	if !hasEvents {
		return s.usageEventsFromSessions(since)
	}
	return events, nil
}

func (s *Store) appendUsageEvent(session storage.Session, usage storage.Usage, total, liveContext int64, createdAt time.Time) error {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	usage.TotalTokens = total
	usage.ContextTokens = liveContext
	if !usage.Countable() {
		return nil
	}
	event := storage.UsageEvent{
		SessionID:     session.ID,
		Runtime:       session.Runtime,
		Agent:         usageAgent(session),
		ModelProvider: session.ModelProvider,
		Model:         session.Model,
		Usage:         usage,
		Source:        storage.UsageEventSourceTurn,
		CreatedAt:     createdAt,
	}
	file, err := os.OpenFile(s.usageEventsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}

func (s *Store) loadUsageEventsSince(since time.Time) ([]storage.UsageEvent, bool, error) {
	file, err := os.Open(s.usageEventsPath())
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	var events []storage.UsageEvent
	var hasEvents bool
	dec := json.NewDecoder(file)
	for {
		var event storage.UsageEvent
		if err := dec.Decode(&event); err == io.EOF {
			return events, hasEvents, nil
		} else if err != nil {
			return nil, false, err
		}
		hasEvents = true
		if !event.CreatedAt.Before(since) {
			events = append(events, event)
		}
	}
}

func (s *Store) usageEventsFromSessions(since time.Time) ([]storage.UsageEvent, error) {
	sessions, err := s.listSessionsLocked(storage.SessionFilter{IncludeChildren: true})
	if err != nil {
		return nil, err
	}
	events := make([]storage.UsageEvent, 0, len(sessions))
	for _, session := range sessions {
		if session.UpdatedAt.Before(since) || !session.Usage.Countable() {
			continue
		}
		events = append(events, storage.UsageEvent{
			SessionID:     session.ID,
			Runtime:       session.Runtime,
			Agent:         usageAgent(session),
			ModelProvider: session.ModelProvider,
			Model:         session.Model,
			Usage:         session.Usage,
			Source:        storage.UsageEventSourceSessionImport,
			CreatedAt:     session.UpdatedAt,
		})
	}
	return events, nil
}

func (s *Store) usageEventsPath() string {
	return filepath.Join(s.root, "usage_events.jsonl")
}

func usageAgent(session storage.Session) string {
	if session.RuntimeRef != nil {
		return session.RuntimeRef.Agent
	}
	return ""
}
