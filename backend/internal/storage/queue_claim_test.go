package storage_test

import (
	"errors"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestQueueClaimChecksSavedSelectionAndKeepsConcurrentEntries(t *testing.T) {
	for _, adapter := range []string{"sqlite", "json"} {
		t.Run(adapter, func(t *testing.T) {
			var store storage.SessionStore
			if adapter == "sqlite" {
				s, err := sqlitestore.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					_ = s.Close()
				})
				store = s
			} else {
				s, err := jsonstore.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				store = s
			}
			session, err := store.CreateSession(storage.CreateSession{Title: "Saved title"})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.QueueTurn(storage.TurnDelivery{SessionID: session.ID, Message: storage.NewQueuedMessage("Initial instruction", nil)}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := store.LoadSession(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			old := snapshot.QueuedMessages[0]
			snapshot.QueuedMessages[0].PlanRequested = true
			if err := store.SaveSession(snapshot); err != nil {
				t.Fatal(err)
			}
			if err := store.ClaimQueuedTurn(session, old); !errors.Is(err, storage.ErrQueueChanged) {
				t.Fatalf("changed selection admitted: %v", err)
			}
			if err := store.QueueTurn(storage.TurnDelivery{SessionID: session.ID, Message: storage.NewQueuedMessage("Later unrelated entry", nil)}); err != nil {
				t.Fatal(err)
			}
			selected := snapshot.QueuedMessages[0]
			snapshot.QueuedMessages = nil
			if err := store.ClaimQueuedTurn(snapshot, selected); err != nil {
				t.Fatal(err)
			}
			claimed, err := store.LoadSession(session.ID)
			if err != nil || claimed.Turn == nil || !claimed.Turn.PendingMessage.PlanRequested || len(claimed.QueuedMessages) != 1 || claimed.QueuedMessages[0].Text != "Later unrelated entry" {
				t.Fatalf("claim overwrote the saved remainder: %+v, %v", claimed, err)
			}
		})
	}
}
