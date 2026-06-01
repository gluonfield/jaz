package sessionevents

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	SessionID string    `json:"session_id"`
	Type      string    `json:"type"`
	Content   string    `json:"content,omitempty"`
	At        time.Time `json:"at"`
}

type Bus struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{}
}

func New() *Bus {
	return &Bus{subs: map[string]map[chan Event]struct{}{}}
}

func (b *Bus) Subscribe(ctx context.Context, sessionID string) <-chan Event {
	ch := make(chan Event, 16)
	b.mu.Lock()
	if b.subs[sessionID] == nil {
		b.subs[sessionID] = map[chan Event]struct{}{}
	}
	b.subs[sessionID][ch] = struct{}{}
	b.mu.Unlock()
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subs[sessionID], ch)
		if len(b.subs[sessionID]) == 0 {
			delete(b.subs, sessionID)
		}
		b.mu.Unlock()
		close(ch)
	}()
	return ch
}

func (b *Bus) Publish(event Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	b.mu.Lock()
	subs := make([]chan Event, 0, len(b.subs[event.SessionID]))
	for ch := range b.subs[event.SessionID] {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}
