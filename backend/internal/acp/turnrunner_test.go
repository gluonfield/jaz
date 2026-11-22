package acp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/turnrunner"
)

func TestManagerImplementsTurnRunnerLifecycle(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	manager := newFakeAgentManager(t, store, t.TempDir(), nil)
	t.Cleanup(manager.Close)

	var runner turnrunner.Runner = manager

	if _, err := runner.StartTurn(t.Context(), turnrunner.TurnRequest{ThreadID: "missing", Message: "   "}); !errors.Is(err, turnrunner.ErrEmptyMessage) {
		t.Fatalf("empty message err = %v, want ErrEmptyMessage", err)
	}
	if _, err := runner.StartTurn(t.Context(), turnrunner.TurnRequest{ThreadID: "", Message: "hi"}); !errors.Is(err, turnrunner.ErrThreadNotFound) {
		t.Fatalf("empty thread err = %v, want ErrThreadNotFound", err)
	}

	spawned, err := manager.Spawn(t.Context(), acp.SpawnRequest{ACPAgent: "fake", Slug: "runner-lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = runner.CancelTurn(context.Background(), turnrunner.CancelRequest{ThreadID: spawned.SessionID, Origin: "user_stop"})
	})

	started, err := runner.StartTurn(t.Context(), turnrunner.TurnRequest{
		ThreadID: spawned.SessionID,
		Message:  "block until cancelled",
		FromUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.ThreadID != spawned.SessionID || started.Disposition != turnrunner.DispositionStarted {
		t.Fatalf("StartTurn = %+v", started)
	}

	if _, err := runner.StartTurn(t.Context(), turnrunner.TurnRequest{
		ThreadID: spawned.SessionID,
		Message:  "concurrent start",
		FromUser: true,
	}); !errors.Is(err, turnrunner.ErrTurnInProgress) {
		t.Fatalf("concurrent StartTurn err = %v, want ErrTurnInProgress", err)
	}

	// Fake agent does not support live steering, so SteerOrQueue without Interrupt queues durably.
	queued, err := runner.SteerOrQueue(t.Context(), turnrunner.TurnRequest{
		ThreadID: spawned.SessionID,
		Message:  "queued follow-up",
		FromUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if queued.Disposition != turnrunner.DispositionQueued {
		t.Fatalf("SteerOrQueue disposition = %q, want %q", queued.Disposition, turnrunner.DispositionQueued)
	}
	stored, err := store.LoadSession(spawned.SessionID)
	if err != nil || len(stored.QueuedMessages) != 1 || stored.QueuedMessages[0].Text != "queued follow-up" {
		t.Fatalf("queued messages = %+v, %v", stored.QueuedMessages, err)
	}

	// SteerOrQueue with Interrupt cancels the running turn and starts the replacement.
	interrupted, err := runner.SteerOrQueue(t.Context(), turnrunner.TurnRequest{
		ThreadID:  spawned.SessionID,
		Message:   "replacement turn",
		FromUser:  true,
		Interrupt: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Disposition != turnrunner.DispositionInterrupted {
		t.Fatalf("SteerOrQueue(interrupt) disposition = %q, want %q", interrupted.Disposition, turnrunner.DispositionInterrupted)
	}
	job, err := manager.Wait(t.Context(), acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil || job.State != acp.StateIdle {
		t.Fatalf("replacement turn = (%+v, %v)", job, err)
	}

	// Start another blocking turn and verify CancelTurn stops it cleanly.
	if _, err := runner.StartTurn(t.Context(), turnrunner.TurnRequest{
		ThreadID: spawned.SessionID,
		Message:  "block until cancelled",
		FromUser: true,
	}); err != nil {
		t.Fatal(err)
	}
	cancelled, err := runner.CancelTurn(t.Context(), turnrunner.CancelRequest{
		ThreadID: spawned.SessionID,
		Origin:   "user_stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Disposition != turnrunner.DispositionCancelled || cancelled.State != acp.StateCancelled {
		t.Fatalf("CancelTurn = %+v", cancelled)
	}
	status, err := manager.Status(spawned.SessionID)
	if err != nil || status.State != acp.StateCancelled {
		t.Fatalf("status after cancel = %+v, %v", status, err)
	}
}
