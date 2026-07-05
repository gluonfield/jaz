package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionQueueActionPreservesInternalMessages(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue-reorder"})
	if err != nil {
		t.Fatal(err)
	}
	internal := storage.NewInternalQueuedMessage("child result")
	internal.ID = "internal"
	public := queuedMessages("first", "second")
	session.QueuedMessages = append([]storage.QueuedMessage{internal}, public...)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	firstID := queuedMessageID(t, store, session.ID, "first")
	secondID := queuedMessageID(t, store, session.ID, "second")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"reorder","ids":["`+secondID+`","`+firstID+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 3 || !loaded.QueuedMessages[0].IsInternal() || queuedTexts(loaded.QueuedMessages) != "child result|second|first" {
		t.Fatalf("stored queue = %#v, want hidden internal preserved before reordered public prompts", loaded.QueuedMessages)
	}
	if public := canonicalSessionResponse(loaded).QueuedMessages; queuedTexts(public) != "second|first" {
		t.Fatalf("public queue = %#v, want reordered public prompts only", public)
	}
}

func TestInternalTurnQueuesWhileParentRunningAndStaysHidden(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue",
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
	session.Status = storage.StatusRunning
	session.QueuedMessages = queuedMessages("public prompt")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}
	srv := &Server{Store: store, ACP: manager}

	if err := srv.StartInternalTurn(context.Background(), session.ID, "child result"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 2 || !loaded.QueuedMessages[0].IsInternal() || loaded.QueuedMessages[0].Text != "child result" {
		t.Fatalf("stored queue = %#v, want internal child result before public prompt", loaded.QueuedMessages)
	}
	if public := canonicalSessionResponse(loaded).QueuedMessages; queuedTexts(public) != "public prompt" {
		t.Fatalf("public queue = %#v, want only public prompt", public)
	}
	manager.mu.Lock()
	internal := manager.internal
	manager.mu.Unlock()
	if internal.Message != "" {
		t.Fatalf("internal turn started while parent running: %#v", internal)
	}
}

func TestInternalTurnRunsBeforeExistingPublicQueue(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue",
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
	session.QueuedMessages = queuedMessages("public prompt")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	}}
	srv := &Server{Store: store, ACP: manager}

	if err := srv.StartInternalTurn(context.Background(), session.ID, "child result"); err != nil {
		t.Fatal(err)
	}
	internal := waitForACPInternal(t, manager, "child result")
	if internal.Session != session.ID {
		t.Fatalf("internal turn = %#v, want session %s", internal, session.ID)
	}
	if sent := sentACPRequest(manager); sent.Message != "" {
		t.Fatalf("public queued prompt ran before internal turn: %#v", sent)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "public prompt" {
		t.Fatalf("queue = %#v, want public prompt left after internal turn", loaded.QueuedMessages)
	}
}

func waitForACPInternal(t *testing.T, manager *fakeACPManager, text string) acp.InternalTurnRequest {
	t.Helper()
	var internal acp.InternalTurnRequest
	waitFor(t, time.Second, func() bool {
		manager.mu.Lock()
		internal = manager.internal
		manager.mu.Unlock()
		return internal.Message == text
	})
	return internal
}
