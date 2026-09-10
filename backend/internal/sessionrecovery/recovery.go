package sessionrecovery

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
)

type Runtime interface {
	ResumeInterruptedTurn(context.Context, string) error
}

func Resume(ctx context.Context, store storage.SessionStore, runtime Runtime, locks *sessionlock.Locks, events *sessionevents.Bus, logger *log.Logger) error {
	sessions, err := store.ListSessions(storage.SessionFilter{Runtime: storage.RuntimeACP, RootOnly: true, Limit: 10})
	if err != nil {
		return err
	}
	for _, candidate := range sessions {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if candidate.Status != storage.StatusInterrupted {
			continue
		}
		unlock := locks.Lock(candidate.ID)
		err := resume(ctx, store, runtime, candidate, events)
		unlock()
		if err != nil {
			logger.Error("resume interrupted chat", "session", candidate.ID, "error", err)
		}
	}
	return ctx.Err()
}

func resume(ctx context.Context, store storage.SessionStore, runtime Runtime, candidate storage.Session, events *sessionevents.Bus) error {
	session, err := store.LoadSession(candidate.ID)
	if err != nil {
		return err
	}
	if session.Status != storage.StatusInterrupted || session.Archived || !session.UpdatedAt.Equal(candidate.UpdatedAt) || ctx.Err() != nil {
		return ctx.Err()
	}
	startCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	err = runtime.ResumeInterruptedTurn(startCtx, session.ID)
	if err != nil && ctx.Err() == nil {
		message := fmt.Sprintf("Could not resume chat after restart: %v", err)
		if saveErr := store.UpdateSessionStatus(session.ID, storage.StatusError, message, time.Time{}); saveErr != nil {
			return fmt.Errorf("%s: %w", message, saveErr)
		}
	}
	events.Publish(sessionevents.Event{SessionID: session.ID, Type: sessionevents.TypeSession})
	return err
}
