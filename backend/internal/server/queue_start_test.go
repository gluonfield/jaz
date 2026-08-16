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

func TestSessionQueueActionStartsPromptWhenIdle(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "queue-start",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateIdle}}
	srv := &Server{Store: store, ACP: manager}
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"append","message":{"text":"start once","plan_requested":true}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	sent := waitForACPSend(t, manager, "start once")
	if sent.Session != session.ID || sent.Completion != acp.CompletionAsync || !sent.PlanRequested {
		t.Fatalf("unexpected queued send %#v", sent)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 0 || loaded.Status != storage.StatusRunning {
		t.Fatalf("session queue=%#v status=%q", loaded.QueuedMessages, loaded.Status)
	}
}
