package sessionrecovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type testRuntime func(context.Context, string) error

func (f testRuntime) ResumeInterruptedTurn(ctx context.Context, id string) error {
	return f(ctx, id)
}

func (f testRuntime) FailInterruptedTurn(context.Context, string, string) error {
	return errors.New("no resume was expected to fail")
}

// failingRuntime ends a turn that cannot resume in error, as the server's
// failed-turn bookkeeping does.
type failingRuntime struct {
	testRuntime
	store storage.SessionStore
}

func (r failingRuntime) FailInterruptedTurn(_ context.Context, id, message string) error {
	return r.store.UpdateSessionStatus(id, storage.StatusError, message, time.Time{})
}

func TestResumeInterruptedBotsAndWorkersAlongsideChats(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	base := time.Now().UTC().Add(-time.Hour)
	var sessions []storage.Session
	for i := range 15 {
		session, err := store.CreateSession(storage.CreateSession{Slug: fmt.Sprintf("chat-%02d", i)})
		if err != nil {
			t.Fatal(err)
		}
		session.Status = storage.StatusInterrupted
		session.LastAttentionAt = base.Add(time.Duration(i) * time.Minute)
		switch i {
		case 4:
			session.SourceType = storage.SourceBot
		case 8:
			session.Status = storage.StatusIdle
		case 9:
			session.Status = storage.StatusError
			session.Error = "provider failure"
		case 12:
			session.SourceType = storage.SourceLoopRun
		case 13:
			session.ParentID = sessions[0].ID
			session.SourceType = storage.SourceBotWorker
		case 14:
			session.Archived = true
		}
		if err := store.SaveSession(session); err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, session)
	}
	var resumed []string
	runtime := testRuntime(func(_ context.Context, id string) error {
		resumed = append(resumed, id)
		return store.UpdateSessionStatus(id, storage.StatusIdle, "", time.Time{})
	})
	locks := sessionlock.New()
	bus := sessionevents.New()
	logger := log.New(io.Discard)
	for range 2 {
		if err := Resume(t.Context(), store, runtime, locks, bus, logger); err != nil {
			t.Fatal(err)
		}
	}
	var want []string
	for _, i := range []int{13, 12, 11, 10, 7, 6, 5, 4, 3, 2, 1, 0} {
		want = append(want, sessions[i].ID)
	}
	if !reflect.DeepEqual(resumed, want) {
		t.Fatalf("resumed = %v, want %v", resumed, want)
	}
}

func TestResumeAParentAfterItsResumedWorkerDrawsItsAttention(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	base := time.Now().UTC().Add(-time.Hour)
	var parent, worker storage.Session
	for i, slug := range []string{"bot", "worker"} {
		session, err := store.CreateSession(storage.CreateSession{Slug: slug})
		if err != nil {
			t.Fatal(err)
		}
		session.Status = storage.StatusInterrupted
		session.SourceType = storage.SourceBot
		session.LastAttentionAt = base.Add(time.Duration(i) * time.Minute)
		if i == 1 {
			session.ParentID, session.SourceType = parent.ID, storage.SourceBotWorker
			worker = session
		} else {
			parent = session
		}
		if err := store.SaveSession(session); err != nil {
			t.Fatal(err)
		}
	}
	var resumed []string
	runtime := testRuntime(func(_ context.Context, id string) error {
		resumed = append(resumed, id)
		// A resumed worker's turn draws its parent's attention as it starts.
		if id == worker.ID {
			if err := store.TouchSessionAttention(parent.ID); err != nil {
				return err
			}
		}
		return store.UpdateSessionStatus(id, storage.StatusIdle, "", time.Time{})
	})
	if err := Resume(t.Context(), store, runtime, sessionlock.New(), sessionevents.New(), log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	if want := []string{worker.ID, parent.ID}; !reflect.DeepEqual(resumed, want) {
		t.Fatalf("resumed = %v, want %v", resumed, want)
	}
}

func TestResumeFailureIsNotRetriedOnNextStartup(t *testing.T) {
	delays := retryDelays
	retryDelays = []time.Duration{0, 0}
	t.Cleanup(func() { retryDelays = delays })
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "failed-recovery"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusInterrupted, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	runtime := failingRuntime{store: store, testRuntime: func(context.Context, string) error {
		attempts++
		return errors.New("provider unavailable")
	}}
	for range 2 {
		if err := Resume(t.Context(), store, runtime, sessionlock.New(), sessionevents.New(), log.New(io.Discard)); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := store.LoadSession(session.ID)
	if err != nil || attempts != 3 || stored.Status != storage.StatusError || stored.Error != "Could not resume chat after restart: provider unavailable" {
		t.Fatalf("attempts = %d, stored = %+v, err = %v", attempts, stored, err)
	}
}

func TestAResumeThatRunsOutOfTimeIsNotTriedAgain(t *testing.T) {
	timeout := resumeTimeout
	resumeTimeout = 10 * time.Millisecond
	t.Cleanup(func() { resumeTimeout = timeout })
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "slow-recovery"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusInterrupted, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	runtime := failingRuntime{store: store, testRuntime: func(ctx context.Context, _ string) error {
		attempts++
		<-ctx.Done()
		return ctx.Err()
	}}
	if err := Resume(t.Context(), store, runtime, sessionlock.New(), sessionevents.New(), log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadSession(session.ID)
	if err != nil || attempts != 1 || stored.Status != storage.StatusError {
		t.Fatalf("attempts = %d, stored = %+v, err = %v", attempts, stored, err)
	}
}

func TestRecoveryShutdownLeavesInterruptedChatResumable(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "cancel-recovery"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusInterrupted, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runtime := testRuntime(func(ctx context.Context, _ string) error {
		cancel()
		return ctx.Err()
	})
	err = Resume(ctx, store, runtime, sessionlock.New(), sessionevents.New(), log.New(io.Discard))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	stored, err := store.LoadSession(session.ID)
	if err != nil || stored.Status != storage.StatusInterrupted || stored.Error != "" {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}
}

// endlessRuntime cannot resume a chat, nor end its turn.
type endlessRuntime struct{}

func (endlessRuntime) ResumeInterruptedTurn(context.Context, string) error {
	return errors.New("provider unavailable")
}

func (endlessRuntime) FailInterruptedTurn(context.Context, string, string) error {
	return errors.New("store unavailable")
}

func TestAChatWhoseTurnCannotEndAfterItsLastTryIsLeftInError(t *testing.T) {
	delays := retryDelays
	retryDelays = []time.Duration{0, 0}
	t.Cleanup(func() { retryDelays = delays })
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "stuck-recovery"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusInterrupted, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := Resume(t.Context(), store, endlessRuntime{}, sessionlock.New(), sessionevents.New(), log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadSession(session.ID)
	if err != nil || stored.Status != storage.StatusError || stored.Error != "Could not resume chat after restart: provider unavailable" {
		t.Fatalf("stored = %+v, %v; want the chat left in error so no later startup tries it", stored, err)
	}
}
