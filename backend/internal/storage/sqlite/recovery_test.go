package sqlite

import (
	"context"
	"io/fs"
	"reflect"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestReopenMarksRunningChatsInterruptedWithoutLosingState(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "crashed"})
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusRunning
	session.Turn = &storage.Turn{PlanRequested: true}
	session.LastAttentionAt = time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	session.QueuedMessages = []storage.QueuedMessage{{ID: "queued", Text: "next request"}}
	session.PendingSteer = &storage.QueuedMessage{ID: "steer", Text: "follow-up"}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	before, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	after, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	before.Status = storage.StatusInterrupted
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("reopen changed session state beyond interruption:\nbefore = %+v\nafter = %+v", before, after)
	}
}

func TestMigrationRecoversOnlyRestartErrors(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, message := range []string{"Server restarted while this thread was still running.", "provider failure"} {
		session, err := store.CreateSession(storage.CreateSession{})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateSessionStatus(session.ID, storage.StatusError, message, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(context.Background(), 48); err != nil {
		t.Fatal(err)
	}
	if err := store.migrate(); err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ListSessions(storage.SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, session := range sessions {
		statuses[session.Error] = session.Status
	}
	if statuses[""] != storage.StatusInterrupted || statuses["provider failure"] != storage.StatusError {
		t.Fatalf("migrated statuses = %v", statuses)
	}
}
