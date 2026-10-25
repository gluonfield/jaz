package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestFailInterruptedLegacySessionWithoutTurnIsTerminalOnce(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "legacy-interrupted"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusInterrupted, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, ACP: &fakeACPManager{}}
	for _, reason := range []string{"Provider session cannot resume", "Late duplicate failure"} {
		if err := srv.FailInterruptedTurn(t.Context(), session.ID, errors.New(reason)); err != nil {
			t.Fatal(err)
		}
		current, err := store.LoadSession(session.ID)
		if err != nil || current.Status != storage.StatusError || current.Turn != nil || current.Error != "Provider session cannot resume" {
			t.Fatalf("legacy failure was lost or overwritten: %+v %v", current, err)
		}
	}
}

func TestInterruptedQueueActionResumesAsAction(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "claimed-archive"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.QueueTurn(storage.TurnDelivery{SessionID: session.ID, Message: storage.QueuedMessage{ID: "archive", Action: storage.QueuedActionArchive}}); err != nil {
		t.Fatal(err)
	}
	session, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimQueuedTurn(session, session.QueuedMessages[0]); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusInterrupted, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{}
	srv := &Server{Store: store, ACP: manager}
	if err := srv.ResumeInterruptedTurn(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	waitForSession(t, store, session.ID, func(session storage.Session) bool { return session.Archived })
	if request := sentACPRequest(manager); request.Message != "" {
		t.Fatalf("archive action became a model prompt: %+v", request)
	}
}
