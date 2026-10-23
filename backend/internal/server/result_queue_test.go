package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestWorkerResultsRemainEvidenceAcrossLaterTurns(t *testing.T) {
	for _, test := range []struct {
		name       string
		firstState string
		laterState string
		laterTurn  storage.Turn
	}{
		{"current failure", acp.StateFailed, "", storage.Turn{}},
		{"failure then running", acp.StateFailed, acp.StateRunning, storage.Turn{}},
		{"failure then success", acp.StateFailed, acp.StateIdle, storage.Turn{}},
		{"failure then new failure", acp.StateFailed, acp.StateFailed, storage.Turn{}},
		{"success then running", acp.StateIdle, acp.StateRunning, storage.Turn{}},
		{"failure then compact", acp.StateFailed, acp.StateIdle, storage.Turn{ActiveOperation: acp.ActiveOperationCompact}},
		{"failure then plan", acp.StateFailed, acp.StateIdle, storage.Turn{PlanRequested: true}},
		{"failure then cancellation", acp.StateFailed, acp.StateCancelled, storage.Turn{}},
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
			if err != nil || len(queued.QueuedMessages) != 1 {
				t.Fatalf("completion did not survive reopen: %+v, %v", queued.QueuedMessages, err)
			}
			original := queued.QueuedMessages[0]
			wantPending := 0
			if test.laterState != "" {
				turn := test.laterTurn
				turn.NotifyParent = !turn.PlanRequested && turn.ActiveOperation == ""
				if err := store.StartSessionTurn(child.ID, turn); err != nil {
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
					if turn.NotifyParent {
						wantPending = 1
					}
				}
			}
			if err := store.FinishSessionTurn(parent.ID, storage.StatusIdle, "", time.Now(), nil); err != nil {
				t.Fatal(err)
			}
			manager := &fakeACPManager{}
			srv.ACP = manager
			session, prompt, ok, err := srv.claimNextTurn(parent.ID)
			if err != nil || !ok || prompt == nil || prompt.Text != original.Text || prompt.From != child.ID {
				t.Fatalf("undelivered worker result lost: %+v, %v", prompt, err)
			}
			if err := srv.startQueuedPrompt(context.Background(), session, *prompt); err != nil {
				t.Fatal(err)
			}
			current, err := store.LoadSession(child.ID)
			if err != nil {
				t.Fatal(err)
			}
			manager.mu.Lock()
			message := manager.internal.Message
			manager.mu.Unlock()
			if !strings.HasPrefix(message, original.Text+"\n\n") ||
				!strings.Contains(message, "Current state of thread design: "+current.Status+".") {
				t.Fatalf("worker result lost its envelope or current state: %s", message)
			}
			saved, err := store.LoadSession(parent.ID)
			if err != nil || saved.Turn.PendingMessage.Text != original.Text || len(saved.QueuedMessages) != wantPending {
				t.Fatalf("worker evidence changed or newer result lost: %+v, %v", saved, err)
			}
		})
	}
}

func TestClaimedWorkerResultUsesCurrentStateOnResume(t *testing.T) {
	for _, typed := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "typed"}[typed], func(t *testing.T) {
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
			if err := store.FinishSessionTurn(child.ID, storage.StatusError, "Authentication required", time.Now(), nil); err != nil {
				t.Fatal(err)
			}
			result := storage.NewInternalQueuedMessage("Authentication required. Base it only on this result; do not say work is still in progress.")
			result.From = child.ID
			if typed {
				result.Text = acp.CompletionPrompt(acp.Job{Slug: child.Slug, State: acp.StateFailed, Error: "Authentication required"}, "the user")
				raw, err := json.Marshal(map[string]any{
					"kind": "internal", "from": child.ID, "text": result.Text,
					"result": map[string]any{"number": 1, "status": storage.StatusError},
				})
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := storage.UnmarshalQueuedMessage(string(raw))
				if err != nil || decoded == nil {
					t.Fatalf("read stored versioned result: %+v, %v", decoded, err)
				}
				result = *decoded
			}
			if err := store.QueueTurn(storage.TurnDelivery{SessionID: parent.ID, Message: result}); err != nil {
				t.Fatal(err)
			}
			manager := &fakeACPManager{}
			srv := &Server{Store: store, ACP: manager, Locks: sessionlock.New()}
			_, claimed, ok, err := srv.claimNextTurn(parent.ID)
			if err != nil || !ok || claimed == nil {
				t.Fatalf("claim result: %v, %v", ok, err)
			}
			if err := store.StartSessionTurn(child.ID, storage.Turn{}); err != nil {
				t.Fatal(err)
			}
			if err := srv.ResumeInterruptedTurn(context.Background(), parent.ID); err != nil {
				t.Fatal(err)
			}
			manager.mu.Lock()
			message := manager.internal.Message
			manager.mu.Unlock()
			if !strings.HasPrefix(message, claimed.Text+"\n\n") || !strings.Contains(message, "Current state of thread worker: running.") {
				t.Fatalf("result lost its envelope or current state: %s", message)
			}
			saved, err := store.LoadSession(parent.ID)
			if err != nil || saved.Turn.PendingMessage.Text != claimed.Text {
				t.Fatalf("provider annotation changed saved result: %+v, %v", saved.Turn, err)
			}
		})
	}
}
