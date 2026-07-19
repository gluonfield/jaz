package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type failingUnreadStore struct {
	storage.Store
	err error
}

func (s failingUnreadStore) SetThreadUnread(string, bool) error { return s.err }

func TestACPTurnFinishedRecordsFeedCompletion(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "finished"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusRunning, "", session.LastAttentionAt); err != nil {
		t.Fatal(err)
	}

	(&Server{Store: store}).HandleACPTurnFinished(context.Background(), acp.Job{
		ID: session.ID, State: acp.StateIdle,
	})

	items, err := store.LoadFeedCompletions()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != session.ID || items[0].CompletedAt.IsZero() {
		t.Fatalf("completions = %#v", items)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusIdle || !loaded.Unread {
		t.Fatalf("completed session = %#v", loaded)
	}
}

func TestSessionSeenActionClearsFeed(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "seen-me"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: "acp_message", Content: "ping"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetThreadUnread(session.ID, true); err != nil {
		t.Fatal(err)
	}

	feed, err := store.LoadFeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 1 {
		t.Fatalf("feed before seen = %d, want 1", len(feed))
	}

	handler := (&Server{Store: store}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/seen", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("seen status = %d, body = %s", res.Code, res.Body.String())
	}

	feed, err = store.LoadFeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 0 {
		t.Fatalf("feed after seen = %d, want 0", len(feed))
	}
}

func TestSessionSeenActionReportsStorageFailure(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "seen-failure"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetThreadUnread(session.ID, true); err != nil {
		t.Fatal(err)
	}
	want := errors.New("unread write failed")
	handler := (&Server{Store: failingUnreadStore{Store: store, err: want}}).Handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/seen", nil))

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("seen status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Unread {
		t.Fatal("failed seen mutation cleared unread state")
	}
}

func TestSessionSeenActionSkipsWriteWhenAlreadySeen(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "already-seen"})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: failingUnreadStore{Store: store, err: errors.New("unexpected write")}}).Handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/seen", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("seen status = %d, body = %s", res.Code, res.Body.String())
	}
}
