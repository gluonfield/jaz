package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestQueuedWorkerResultsReconcileLaterTurns(t *testing.T) {
	for _, test := range []struct {
		name       string
		firstState string
		laterState string
		wantResult string
	}{
		{"current failure", acp.StateFailed, "", "Authentication required"},
		{"failure then running", acp.StateFailed, acp.StateRunning, ""},
		{"failure then success", acp.StateFailed, acp.StateIdle, "Revised design"},
		{"failure then new failure", acp.StateFailed, acp.StateFailed, "Capacity exhausted"},
		{"success then running", acp.StateIdle, acp.StateRunning, "Original design"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := sqlitestore.New(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			parent, err := store.CreateSession(storage.CreateSession{
				Slug: "designer", RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: acp.AgentCodex},
			})
			if err != nil {
				t.Fatal(err)
			}
			child, err := store.CreateSession(storage.CreateSession{Slug: "design", ParentID: parent.ID})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.StartSessionTurn(parent.ID, storage.Turn{}); err != nil {
				t.Fatal(err)
			}
			if err := store.StartSessionTurn(child.ID, storage.Turn{NotifyParent: true}); err != nil {
				t.Fatal(err)
			}
			srv := &Server{Store: store, Locks: sessionlock.New()}
			first := acp.Job{ID: child.ID, Slug: child.Slug, State: test.firstState, Assistant: "Original design"}
			if test.firstState == acp.StateFailed {
				first.Error = "Authentication required"
			}
			if err := srv.finishTurn(first); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = sqlitestore.New(root)
			if err != nil {
				t.Fatal(err)
			}
			srv.Store = store
			queued, err := store.LoadSession(parent.ID)
			if err != nil || len(queued.QueuedMessages) != 1 || queued.QueuedMessages[0].Result == nil || queued.QueuedMessages[0].Result.Number != 1 {
				t.Fatalf("completion provenance did not survive reopen: %+v, %v", queued.QueuedMessages, err)
			}
			if test.laterState != "" {
				if err := store.StartSessionTurn(child.ID, storage.Turn{NotifyParent: true}); err != nil {
					t.Fatal(err)
				}
				if test.laterState != acp.StateRunning {
					later := acp.Job{ID: child.ID, Slug: child.Slug, State: test.laterState, Assistant: "Revised design"}
					if test.laterState == acp.StateFailed {
						later.Error = "Capacity exhausted"
					}
					if err := srv.finishTurn(later); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := store.FinishSessionTurn(parent.ID, storage.StatusIdle, "", time.Now(), nil); err != nil {
				t.Fatal(err)
			}
			_, prompt, ok, err := srv.claimNextTurn(parent.ID)
			if err != nil {
				t.Fatal(err)
			}
			if test.wantResult == "" {
				if ok || prompt != nil {
					t.Fatalf("superseded failure reached parent: %+v", prompt)
				}
			} else if !ok || prompt == nil || !strings.Contains(prompt.Text, test.wantResult) {
				t.Fatalf("result = %+v, want %q", prompt, test.wantResult)
			}
			saved, err := store.LoadSession(parent.ID)
			if err != nil || len(saved.QueuedMessages) != 0 {
				t.Fatalf("result queue not drained: %+v, %v", saved.QueuedMessages, err)
			}
		})
	}
}

func TestLegacyWorkerResultIsHistoricalAndKeepsCurrentState(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	parent, err := store.CreateSession(storage.CreateSession{Slug: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{Slug: "worker", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(child.ID, storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	legacy := storage.NewInternalQueuedMessage("Authentication required. Base it only on this result; do not say work is still in progress.")
	legacy.From = child.ID
	if err := store.QueueTurn(storage.TurnDelivery{SessionID: parent.ID, Message: legacy}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{}
	srv := &Server{Store: store, ACP: manager, Locks: sessionlock.New()}
	session, prompt, ok, err := srv.claimNextTurn(parent.ID)
	if err != nil || !ok {
		t.Fatalf("claim legacy result: %v, %v", ok, err)
	}
	if err := srv.startQueuedPrompt(context.Background(), session, *prompt); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	message := manager.internal.Message
	manager.mu.Unlock()
	if !strings.Contains(message, "current status is running") || !strings.Contains(message, "<completed_turn_result>\n"+legacy.Text+"\n</completed_turn_result>") {
		t.Fatalf("legacy result lost its evidence boundary or current state: %s", message)
	}
	saved, err := store.LoadSession(parent.ID)
	if err != nil || saved.Turn.PendingMessage.Text != legacy.Text {
		t.Fatalf("provider annotation changed saved result: %+v, %v", saved.Turn, err)
	}
}
