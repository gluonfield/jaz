package server

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

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
