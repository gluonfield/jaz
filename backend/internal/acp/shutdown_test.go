package acp_test

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestCloseMarksRunningTurnAsServerShutdownWithoutTransportFailure(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), nil)
	finished := make(chan struct{})
	releaseFinished := make(chan struct{})
	manager.TurnFinished = func(context.Context, acp.Job) {
		close(finished)
		<-releaseFinished
	}
	waitForState := func(sessionID, want string) acp.Job {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		var last acp.Job
		var lastErr error
		for time.Now().Before(deadline) {
			last, lastErr = manager.Status(sessionID)
			if lastErr == nil && last.State == want {
				return last
			}
			time.Sleep(10 * time.Millisecond)
		}
		if lastErr != nil {
			t.Fatalf("acp status error = %v", lastErr)
		}
		t.Fatalf("acp status = %q, want %q", last.State, want)
		return acp.Job{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-close"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Send(ctx, acp.SendRequest{
		Session:    spawned.SessionID,
		Message:    "block until cancelled",
		Completion: acp.CompletionInline,
	}); err != nil {
		t.Fatal(err)
	}
	waitForState(spawned.SessionID, acp.StateRunning)

	closed := make(chan struct{})
	go func() {
		manager.Close()
		close(closed)
	}()
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("turn did not finish during manager close")
	}
	select {
	case <-closed:
		t.Fatal("manager close returned before turn completion")
	default:
	}
	close(releaseFinished)
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("manager close did not return after turn completion")
	}

	state := waitForState(spawned.SessionID, acp.StateCancelled)
	if state.StopReason != acp.StopReasonServerShutdown || state.Error != "" {
		t.Fatalf("state stop/error = %q/%q, want server_shutdown with no error", state.StopReason, state.Error)
	}
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != storage.StatusIdle || session.Error != "" {
		t.Fatalf("session status/error = %q/%q, want idle with no error", session.Status, session.Error)
	}
}

func TestCloseWaitsForFinishingIdleTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), nil)
	finished := make(chan struct{})
	releaseFinished := make(chan struct{})
	manager.TurnFinished = func(context.Context, acp.Job) {
		close(finished)
		<-releaseFinished
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-close-idle"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Send(ctx, acp.SendRequest{
		Session:    spawned.SessionID,
		Message:    "say hello",
		Completion: acp.CompletionInline,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("turn did not reach completion")
	}

	closed := make(chan struct{})
	go func() {
		manager.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("manager close returned before idle turn completion")
	default:
	}
	close(releaseFinished)
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("manager close did not return after idle turn completion")
	}

	state, err := manager.Status(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != acp.StateIdle || state.StopReason == acp.StopReasonServerShutdown || state.Error != "" {
		t.Fatalf("state/stop/error = %q/%q/%q, want idle without shutdown error", state.State, state.StopReason, state.Error)
	}
}
