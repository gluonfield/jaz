package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestACPStreamQueuesPromptReservedByRunningTurn(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "already-running",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusRunning
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateIdle}}
	server := &Server{Store: store, ACP: manager}
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/sessions/"+session.ID+"/messages:stream",
		strings.NewReader(`{"message":"also loader not visible","contexts":[{"type":"selection","text":"loader"}],"plan_requested":true}`),
	)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), `"type":"error"`) {
		t.Fatalf("response = %d %s", res.Code, res.Body.String())
	}
	if manager.sent.Message != "" {
		t.Fatalf("reserved turn was sent concurrently: %#v", manager.sent)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 1 {
		t.Fatalf("queue = %#v", loaded.QueuedMessages)
	}
	queued := loaded.QueuedMessages[0]
	if queued.Text != "also loader not visible" || !queued.PlanRequested || len(queued.Contexts) != 1 || queued.Contexts[0].Text != "loader" {
		t.Fatalf("queued prompt = %#v", queued)
	}
}

func TestACPStreamPublishesMessageRefreshAfterAccept(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "accepted-message-refresh",
		Title:   "Accepted message refresh",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateIdle}}
	server := &Server{Store: store, Events: events, ACP: manager}
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"show this now"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	server.Handler().ServeHTTP(res, req)

	deadline := time.After(time.Second)
	for {
		select {
		case event := <-sub:
			if event.Type == "assistant" {
				return
			}
		case <-deadline:
			t.Fatal("accepted prompt did not publish a message refresh")
		}
	}
}

func TestBeginACPTurnClearsStaleError(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "acp-retry",
		Title:   "Existing thread",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusError
	session.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	server := &Server{
		Store:  store,
		Events: events,
		ACP:    &fakeACPManager{utilityText: `{"title":"Continue Thread"}`},
	}
	if _, err := server.beginACPTurn(context.Background(), session, "continue"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusRunning || loaded.Error != "" {
		t.Fatalf("loaded status/error = %q/%q, want running with no error", loaded.Status, loaded.Error)
	}
	expectSessionChangedEvent(t, sub, session.ID)
}

func expectSessionChangedEvent(t *testing.T, sub <-chan sessionevents.Event, sessionID string) {
	t.Helper()
	select {
	case event := <-sub:
		if event.SessionID != sessionID || event.Type != sessionevents.TypeSession {
			t.Fatalf("session event = %#v, want session event for %s", event, sessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session event")
	}
}
