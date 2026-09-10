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

func TestResumeOnlyInterruptedChatsInLatestTen(t *testing.T) {
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
		case 8:
			session.Status = storage.StatusIdle
		case 9:
			session.Status = storage.StatusError
			session.Error = "provider failure"
		case 12:
			session.SourceType = storage.SourceLoopRun
		case 13:
			session.ParentID = sessions[0].ID
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
	for _, i := range []int{11, 10, 7, 6, 5, 4, 3, 2} {
		want = append(want, sessions[i].ID)
	}
	if !reflect.DeepEqual(resumed, want) {
		t.Fatalf("resumed = %v, want %v", resumed, want)
	}
}

func TestResumeFailureIsNotRetriedOnNextStartup(t *testing.T) {
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
	runtime := testRuntime(func(context.Context, string) error {
		attempts++
		return errors.New("provider unavailable")
	})
	for range 2 {
		if err := Resume(t.Context(), store, runtime, sessionlock.New(), sessionevents.New(), log.New(io.Discard)); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := store.LoadSession(session.ID)
	if err != nil || attempts != 1 || stored.Status != storage.StatusError || stored.Error != "Could not resume chat after restart: provider unavailable" {
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
