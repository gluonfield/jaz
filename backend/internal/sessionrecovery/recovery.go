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
	// FailInterruptedTurn ends a chat's interrupted turn that cannot resume
	// as a turn that failed while it ran ends.
	FailInterruptedTurn(ctx context.Context, id, message string) error
}

// retryDelays are the waits before each further try at the chats whose resume
// failed, as when an agent's process dies on its way up. A chat that fails its
// last try ends its turn as failed, so no later startup tries it again.
var retryDelays = []time.Duration{2 * time.Second, 8 * time.Second}

// resumeTimeout bounds one try.
var resumeTimeout = 2 * time.Minute

func Resume(ctx context.Context, store storage.SessionStore, runtime Runtime, locks *sessionlock.Locks, events *sessionevents.Bus, logger *log.Logger) error {
	sessions, err := store.ListSessions(storage.SessionFilter{Runtime: storage.RuntimeACP, IncludeChildren: true, IncludeSourced: true})
	if err != nil {
		return err
	}
	var pending []string
	for _, candidate := range sessions {
		if candidate.Status == storage.StatusInterrupted && resumable(candidate.SourceType) {
			pending = append(pending, candidate.ID)
		}
	}
	for attempt := 0; len(pending) > 0; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelays[attempt-1]):
			}
		}
		var failed []string
		for _, id := range pending {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			unlock := locks.Lock(id)
			retry, err := resume(ctx, store, runtime, id, attempt == len(retryDelays))
			unlock()
			switch {
			case retry:
				logger.Warn("resume interrupted chat failed; trying again", "session", id, "error", err)
				failed = append(failed, id)
			case err != nil:
				logger.Error("resume interrupted chat", "session", id, "error", err)
				if ctx.Err() == nil {
					giveUp(ctx, store, runtime, id, err)
				}
			}
			events.Publish(sessionevents.Event{SessionID: id, Type: sessionevents.TypeSession})
		}
		pending = failed
	}
	return ctx.Err()
}

// resume tries to resume one chat's interrupted turn and reports whether its
// failure is worth another try. A try that used its whole time is not.
func resume(ctx context.Context, store storage.SessionStore, runtime Runtime, id string, last bool) (bool, error) {
	session, err := store.LoadSession(id)
	if err != nil {
		return false, err
	}
	if session.Status != storage.StatusInterrupted || session.Archived || ctx.Err() != nil {
		return false, ctx.Err()
	}
	startCtx, cancel := context.WithTimeout(ctx, resumeTimeout)
	defer cancel()
	err = runtime.ResumeInterruptedTurn(startCtx, session.ID)
	return err != nil && !last && startCtx.Err() == nil, err
}

// giveUp ends the turn of a chat that cannot resume as failed, so whoever
// waits on it hears; if even that fails, the chat is left in error.
func giveUp(ctx context.Context, store storage.SessionStore, runtime Runtime, id string, cause error) {
	message := fmt.Sprintf("Could not resume chat after restart: %v", cause)
	if err := runtime.FailInterruptedTurn(ctx, id, message); err != nil {
		_ = store.UpdateSessionStatus(id, storage.StatusError, message, time.Time{})
	}
}

// resumable reports whether chats with this source resume after a restart:
// the user's own chats, the threads bots work in and routine runs.
func resumable(sourceType string) bool {
	switch sourceType {
	case "", storage.SourceBot, storage.SourceBotWorker, storage.SourceBotMember, storage.SourceLoopRun:
		return true
	}
	return false
}
