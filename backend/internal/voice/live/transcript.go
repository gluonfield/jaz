package live

import (
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type TranscriptStore interface {
	LoadSession(string) (storage.Session, error)
	storage.SessionEventAppender
}

type Transcript struct {
	store TranscriptStore
	bus   *sessionevents.Bus
}

func NewTranscript(store TranscriptStore, bus *sessionevents.Bus) *Transcript {
	return &Transcript{store: store, bus: bus}
}

func (s *Transcript) Append(sessionID string, messages []sessionevents.VoiceMessage) error {
	if len(messages) == 0 || len(messages) > 32 {
		return fmt.Errorf("%w: expected 1–32 spoken messages", ErrInput)
	}
	if _, err := s.store.LoadSession(sessionID); err != nil {
		return err
	}
	events := make([]sessionevents.Event, 0, len(messages))
	for _, message := range messages {
		message.ID = strings.TrimSpace(message.ID)
		message.CallID = strings.TrimSpace(message.CallID)
		message.Text = strings.TrimSpace(message.Text)
		if message.ID == "" || message.CallID == "" || len(message.ID) > 128 || len(message.CallID) > 128 || message.Text == "" || len(message.Text) > 65536 || (message.Role != "user" && message.Role != "assistant") {
			return fmt.Errorf("%w: invalid spoken message", ErrInput)
		}
		if message.At.IsZero() {
			message.At = time.Now().UTC()
		}
		events = append(events, sessionevents.Event{
			SessionID: sessionID, Type: sessionevents.TypeVoiceMessage, Voice: &message,
			ProjectionKey: "voice:" + sessionID + ":" + message.CallID + ":" + message.ID,
			ProjectionOp:  sessionevents.ProjectionReplace,
		})
	}
	if err := s.store.AppendSessionEvents(sessionID, events...); err != nil {
		return err
	}
	for _, event := range events {
		s.bus.Publish(event)
	}
	return nil
}
