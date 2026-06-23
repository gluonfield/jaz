package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/threads"
)

func TestThreadSearchHandler(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       "search-route",
		Title:      "Search route",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: "codex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(session.ID, storage.Message{Role: "user", Content: "Find the command palette thread."}); err != nil {
		t.Fatal(err)
	}
	handler := NewThreadSearchHandler(threads.NewService(sqlitestore.NewSearchQueries(store), store))

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/search/threads?q=palette", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("search status = %d, body = %s", res.Code, res.Body.String())
	}
	var got threadSearchResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0].ThreadID != session.ID {
		t.Fatalf("results = %#v, want session %s", got.Results, session.ID)
	}
	if got.Results[0].ThreadAgent != "codex" {
		t.Fatalf("thread agent = %q, want codex", got.Results[0].ThreadAgent)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/search/threads?q=palette&limit=-1", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for malformed limit, got %d", res.Code)
	}
}
