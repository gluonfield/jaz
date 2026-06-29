package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestACPBackedSessionAllowsContextWithoutText(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-context-only",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	}}

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/sessions/"+session.ID+"/messages:stream",
		strings.NewReader(`{"contexts":[{"type":"selection","text":"selected text"}]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.sent.Message != "" || len(manager.sent.Contexts) != 1 || manager.sent.Contexts[0].Text != "selected text" {
		t.Fatalf("send request = %#v", manager.sent)
	}
}
