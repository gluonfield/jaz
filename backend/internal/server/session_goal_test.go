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

func TestACPBackedSessionAllowsJazGoalWithoutNativeCapability(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "claude-goal",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     acp.AgentClaude,
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"hi","goal_requested":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	manager := &fakeACPManager{job: acp.Job{State: acp.StateIdle}}
	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !manager.sent.GoalRequested {
		t.Fatalf("goal request was not sent to acp manager: %#v", manager.sent)
	}
}

func TestACPBackedCodexSessionForwardsJazGoalRequest(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "old-codex-goal",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     acp.AgentCodex,
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"hi","goal_requested":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	manager := &fakeACPManager{job: acp.Job{State: acp.StateIdle}}
	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !manager.sent.GoalRequested {
		t.Fatalf("goal request was not sent to acp manager: %#v", manager.sent)
	}
}
