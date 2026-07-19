package server

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBeginACPTurnClearsStaleError(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "acp-retry",
		Title:   "Existing thread",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusError
	session.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	server := &Server{
		Store:  store,
		Events: events,
		ACP:    &fakeACPManager{utilityText: `{"title":"Continue Thread"}`},
	}
	if _, err := server.beginACPTurn(context.Background(), session, "continue"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusRunning || loaded.Error != "" {
		t.Fatalf("loaded status/error = %q/%q, want running with no error", loaded.Status, loaded.Error)
	}
	expectSessionChangedEvent(t, sub, session.ID)
}

func expectSessionChangedEvent(t *testing.T, sub <-chan sessionevents.Event, sessionID string) {
	t.Helper()
	select {
	case event := <-sub:
		if event.SessionID != sessionID || event.Type != sessionevents.TypeSession {
			t.Fatalf("session event = %#v, want session event for %s", event, sessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session event")
	}
}
