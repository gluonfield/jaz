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

func TestBeginACPTurnGeneratesTitleFromFirstMessage(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:          "first-message-title",
		Runtime:       storage.RuntimeACP,
		ModelProvider: "codex",
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: "codex",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{utilityText: `{"title":"OAuth Callback Fix"}`}
	server := &Server{Store: store, ACP: manager}

	if _, err := server.beginACPTurn(context.Background(), session, "Please fix the OAuth callback route because the redirect drops state"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(2 * time.Second)
	for {
		loaded, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Title == "OAuth Callback Fix" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("title = %q, want generated title", loaded.Title)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if manager.utilityPrompt.ACPAgent != acp.AgentCodex {
		t.Fatalf("utility prompt request = %#v", manager.utilityPrompt)
	}
	if manager.utilityPrompt.ReasoningEffort != "none" {
		t.Fatalf("utility prompt reasoning effort = %q, want none", manager.utilityPrompt.ReasoningEffort)
	}
}

func TestMessageStreamGeneratesTitleVisibleToUI(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "stream-title",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: "codex",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:        session.ID,
		Slug:      session.Slug,
		State:     acp.StateIdle,
		Assistant: "done",
	}, utilityText: `{"title":"Stream Route Title"}`}
	server := &Server{Store: store, ACP: manager}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"Please repair generated thread names in the sidebar"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.sent.Message != "Please repair generated thread names in the sidebar" {
		t.Fatalf("send request = %#v", manager.sent)
	}
	deadline := time.After(2 * time.Second)
	for {
		loaded, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Title == "Stream Route Title" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("title = %q, want generated title", loaded.Title)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestGeneratedSessionTitlePublishesSessionEvent(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "title-event",
		Title:   "Please rename this generated session",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	server := &Server{
		Store:  store,
		Events: events,
		ACP:    &fakeACPManager{utilityText: `{"title":"Generated Sidebar Title"}`},
	}

	updated := server.generateAndSaveSessionTitle(context.Background(), session, "Please rename this generated session")

	if updated.Title != "Generated Sidebar Title" {
		t.Fatalf("title = %q, want generated title", updated.Title)
	}
	expectSessionChangedEvent(t, sub, session.ID)
}
