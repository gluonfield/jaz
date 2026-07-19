package sqlite

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestThreadReadsDoNotWaitForWriterMutex(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "concurrent-reads"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveSetting("agents", "config", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}

	store.writeMu.Lock()
	done := make(chan error, 1)
	go func() {
		if _, err := store.LoadSession(session.ID); err != nil {
			done <- err
			return
		}
		if _, err := store.ListSessions(storage.SessionFilter{}); err != nil {
			done <- err
			return
		}
		if _, err := store.LoadMessageRecords(session.ID); err != nil {
			done <- err
			return
		}
		if _, err := store.LoadSessionEvents(session.ID); err != nil {
			done <- err
			return
		}
		_, err := store.LoadSetting("agents", "config")
		done <- err
	}()
	select {
	case err := <-done:
		store.writeMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		store.writeMu.Unlock()
		t.Fatal("thread reads waited for the writer mutex")
	}
}

func TestWriterDoesNotWaitForSQLiteReader(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "wal-reader"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM threads`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- store.SetThreadUnread(session.ID, true) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("writer waited for a SQLite reader")
	}
}
