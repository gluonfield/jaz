package acp

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// unsaved stores no threads, as for jobs a test makes up.
type unsaved struct{ Store }

func (unsaved) LoadSession(id string) (storage.Session, error) {
	return storage.Session{}, storage.ErrSessionNotFound
}

func waitingThreads() (*Manager, *jobState, *jobState) {
	manager := NewManager(unsaved{}, Config{}, nil)
	manager.Events = sessionevents.New()
	first := &jobState{Job: Job{ID: "first", State: StateRunning}}
	second := &jobState{Job: Job{ID: "second", State: StateRunning}}
	manager.addJob(first, nil)
	manager.addJob(second, nil)
	return manager, first, second
}

func TestWaitThreadsZeroIsImmediateAndTimeoutLeavesWorkRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager, _, _ := waitingThreads()
		for _, timeout := range []time.Duration{0, time.Second} {
			start := time.Now()
			result, err := manager.WaitThreads(context.Background(), []string{"first", "second"}, timeout)
			if err != nil || len(result.Threads) != 2 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if time.Since(start) != timeout {
				t.Fatalf("elapsed=%v want=%v", time.Since(start), timeout)
			}
			for _, job := range result.Threads {
				if job.State != StateRunning {
					t.Fatalf("wait stopped work: %#v", job)
				}
			}
		}
	})
}

func TestWaitThreadsWakesOnAnyCompletionOrAttentionButNotCommentary(t *testing.T) {
	for _, attention := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			manager, _, second := waitingThreads()
			completed := make(chan ThreadResults, 1)
			go func() {
				result, err := manager.WaitThreads(context.Background(), []string{"first", "second"}, time.Minute)
				if err != nil {
					t.Errorf("wait: %v", err)
				}
				completed <- result
			}()
			synctest.Wait()
			manager.Events.Publish(sessionevents.Event{SessionID: "second", Type: sessionevents.TypeACPMessage, Content: "still working"})
			synctest.Wait()
			select {
			case <-completed:
				t.Fatal("commentary woke wait")
			default:
			}
			second.mu.Lock()
			if attention {
				second.Permissions = []sessionevents.ACPPermission{{ID: "question", Status: "pending"}}
			} else {
				second.State = StateIdle
				second.Assistant = "finished second"
			}
			second.mu.Unlock()
			manager.Events.Publish(sessionevents.Event{SessionID: "second", Type: "acp"})
			synctest.Wait()
			select {
			case result := <-completed:
				if len(result.Threads) != 2 || result.Threads[0].State != StateRunning {
					t.Fatalf("lost other target: %#v", result)
				}
				if attention && len(result.Threads[1].Permissions) != 1 {
					t.Fatal("missing pending input")
				}
				if !attention && result.Threads[1].Assistant != "finished second" {
					t.Fatal("missing completed answer")
				}
			default:
				t.Fatal("wait did not wake on second target")
			}
		})
	}
}

func TestWaitThreadsCancellationAndBounds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager, first, _ := waitingThreads()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := manager.WaitThreads(ctx, []string{"first"}, time.Minute)
			done <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
		if first.Snapshot().State != StateRunning {
			t.Fatal("canceling observation canceled work")
		}
		for _, refs := range [][]string{nil, make([]string, 9)} {
			if _, err := manager.WaitThreads(context.Background(), refs, 0); err == nil {
				t.Fatal("accepted invalid target count")
			}
		}
	})
}

func TestWaitThreadsRestoresSavedResultsAndReportsMissingTargets(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{
		Slug: "saved-thread", Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: AgentCodex, SessionID: "provider-thread"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "Persisted answer", ACP: &sessionevents.ACPEvent{ID: session.ID}},
		sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{ID: session.ID, State: StateIdle}},
	); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	result, err := manager.WaitThreads(t.Context(), []string{session.Slug, "missing"}, 0)
	if err != nil || len(result.Threads) != 1 || result.Errors["missing"] == "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if got := result.Threads[0]; got.ID != session.ID || got.Assistant != "Persisted answer" || got.State != StateNotRunning {
		t.Fatalf("saved result=%#v", got)
	}
}
