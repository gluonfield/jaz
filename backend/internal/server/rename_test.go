package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestRenameSessionUpdatesTitle(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSession(storage.CreateSession{Slug: "chat", Title: "Old title"})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store, ACP: &fakeACPManager{spawnStore: store}}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.ID+"/rename", strings.NewReader(`{"title":"  New title  "}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var session storage.Session
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Title != "New title" {
		t.Fatalf("title = %q, want trimmed %q", session.Title, "New title")
	}
	if session.Slug != created.Slug {
		t.Fatalf("slug = %q, want unchanged %q (rename must not move the slug)", session.Slug, created.Slug)
	}
}

func TestRenameSessionRejectsEmptyTitle(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSession(storage.CreateSession{Slug: "chat", Title: "Keep me"})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store, ACP: &fakeACPManager{spawnStore: store}}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.ID+"/rename", strings.NewReader(`{"title":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", res.Code, res.Body.String())
	}
	reloaded, err := store.LoadSession(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Title != "Keep me" {
		t.Fatalf("title = %q, want unchanged %q", reloaded.Title, "Keep me")
	}
}
