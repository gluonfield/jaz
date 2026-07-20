package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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

type completionObservingStore struct {
	storage.Store
	called chan string
}

func (s completionObservingStore) CompleteSession(id string, at time.Time) error {
	s.called <- id
	return s.Store.CompleteSession(id, at)
}

type gatedSessionLocker struct {
	once    sync.Once
	entered chan string
	release <-chan struct{}
}

func (l *gatedSessionLocker) Lock(id string) func() {
	l.once.Do(func() {
		l.entered <- id
		<-l.release
	})
	return func() {}
}

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

func TestACPTurnFinishedSerializesThroughSessionLock(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "serialized-finish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusRunning, "", session.LastAttentionAt); err != nil {
		t.Fatal(err)
	}

	entered := make(chan string)
	release := make(chan struct{})
	completed := make(chan string, 1)
	server := &Server{
		Store: completionObservingStore{Store: store, called: completed},
		Locks: &gatedSessionLocker{entered: entered, release: release},
	}
	done := make(chan struct{})
	go func() {
		server.HandleACPTurnFinished(context.Background(), acp.Job{ID: session.ID, State: acp.StateIdle})
		close(done)
	}()

	select {
	case id := <-entered:
		if id != session.ID {
			t.Fatalf("locked session = %q, want %q", id, session.ID)
		}
	case id := <-completed:
		t.Fatalf("session %q completed before acquiring its lock", id)
	case <-time.After(time.Second):
		t.Fatal("completion did not acquire the session lock")
	}
	select {
	case id := <-completed:
		t.Fatalf("session %q completed while its lock was blocked", id)
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completion did not finish after the lock was released")
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
