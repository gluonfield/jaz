package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestCreateACPSessionUsesTitleForInitialSlug(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{spawnStore: store}

	body := `{"runtime":"acp","agent":"claude","title":"Repair thread title slugs","directory":"repo","worktree":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.created.Title != "Repair thread title slugs" {
		t.Fatalf("create request = %#v, want title forwarded", manager.created)
	}
	var session storage.Session
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Slug != "repair-thread-title-slugs" {
		t.Fatalf("slug = %q, want task-derived slug", session.Slug)
	}
	if session.ModelProvider != acp.AgentClaude {
		t.Fatalf("model provider = %q, want %q", session.ModelProvider, acp.AgentClaude)
	}
}
