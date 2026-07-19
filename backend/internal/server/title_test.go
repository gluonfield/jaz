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

	waitForSessionTitle(t, store, session.ID, "OAuth Callback Fix")
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
	waitForSessionTitle(t, store, session.ID, "Stream Route Title")
}

func TestStatusUpdatePreservesGeneratedTitle(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "stale-status-title",
		Title:   "Fallback Title",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Store: store}

	if err := store.UpdateSessionTitle(session.ID, "Generated Title"); err != nil {
		t.Fatal(err)
	}
	server.setSessionStatus(session, storage.StatusIdle)

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "Generated Title" {
		t.Fatalf("title = %q, want generated title", loaded.Title)
	}
	if loaded.Status != storage.StatusIdle {
		t.Fatalf("status = %q, want idle", loaded.Status)
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

func TestGeneratedSessionTitlePreservesManualTitle(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "manual-title",
		Title:   "Fallback Title",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionTitle(session.ID, "Fallback Title"); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		Store: store,
		ACP:   &fakeACPManager{utilityText: `{"title":"Generated Title"}`},
	}

	updated := server.generateAndSaveSessionTitle(context.Background(), session, "Fallback Title")

	if updated.Title != "Fallback Title" || !updated.ManualTitle {
		t.Fatalf("title/manual = %q/%v, want manual fallback", updated.Title, updated.ManualTitle)
	}
}

func TestShouldGenerateTitleOnlyFromFirstMessageThenLocks(t *testing.T) {
	first := storage.Session{Title: titleFromMessage("Fix the OAuth callback route")}
	if !shouldGenerateTitleFromMessage(first, "Fix the OAuth callback route", nil) {
		t.Fatal("first message placeholder should still generate a title")
	}
	locked := storage.Session{Title: "OAuth Callback Fix", TitleLocked: true}
	if shouldGenerateTitleFromMessage(locked, "continue", nil) {
		t.Fatal("a locked title must never regenerate, even with empty history")
	}
	generated := storage.Session{Title: "OAuth Callback Fix"}
	if shouldGenerateTitleFromMessage(generated, "continue", nil) {
		t.Fatal("a later message must not replace an existing non-placeholder title")
	}
	manual := storage.Session{Title: "My Title", ManualTitle: true}
	if shouldGenerateTitleFromMessage(manual, "My Title", nil) {
		t.Fatal("a manual title must never regenerate")
	}
}

func TestGeneratedSessionTitleLocksTitle(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "lock-on-generate",
		Title:   "Fix the redirect",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		Store: store,
		ACP:   &fakeACPManager{utilityText: `{"title":"Redirect State Fix"}`},
	}

	server.generateAndSaveSessionTitle(context.Background(), session, "Fix the redirect")

	locked, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if locked.Title != "Redirect State Fix" || !locked.TitleLocked {
		t.Fatalf("title/locked = %q/%v, want generated and locked", locked.Title, locked.TitleLocked)
	}
	// A runtime title push must not override the locked title.
	if _, updated, err := store.UpdateSessionTitleFromRuntime(session.ID, "Runtime Title"); err != nil || updated {
		t.Fatalf("runtime title override updated=%v err=%v, want no update", updated, err)
	}
	final, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Title != "Redirect State Fix" {
		t.Fatalf("title = %q, want unchanged locked title", final.Title)
	}
}

func waitForSessionTitle(t *testing.T, store storage.SessionStore, sessionID, want string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		loaded, err := store.LoadSession(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Title == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("title = %q, want %q", loaded.Title, want)
		case <-time.After(20 * time.Millisecond):
		}
	}
}
