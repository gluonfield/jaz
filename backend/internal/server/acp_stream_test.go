package server

import (
	"context"
	"testing"

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

	if _, err := (&Server{Store: store, Events: events}).beginACPTurn(context.Background(), session, "continue"); err != nil {
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
