package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// The seen action must reach the store's FeedStore seam and clear the thread
// from the feed query.
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
