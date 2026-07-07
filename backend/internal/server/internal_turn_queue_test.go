package server

import (
	"context"
	"errors"
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

func TestSessionQueueSteerCannotClaimInternalMessage(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "internal-steer",
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
	internal := storage.NewInternalQueuedMessage("child result")
	internal.ID = "internal"
	session.Status = storage.StatusRunning
	session.QueuedMessages = append([]storage.QueuedMessage{internal}, queuedMessages("public prompt")...)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","id":"internal"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "child result|public prompt" || !loaded.QueuedMessages[0].IsInternal() || loaded.PendingSteer != nil {
		t.Fatalf("session queue=%#v pending=%#v, want internal message untouched", loaded.QueuedMessages, loaded.PendingSteer)
	}
	if sent := sentACPRequest(manager); sent.Message != "" {
		t.Fatalf("internal queue entry was steered: %#v", sent)
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

func TestSessionQueueSteerRestoresPublicPromptAfterInternalPrefix(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "internal-prefix-steer-fail",
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
	session.QueuedMessages = append([]storage.QueuedMessage{storage.NewInternalQueuedMessage("child result")}, queuedMessages("first", "second", "third")...)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{
		job: acp.Job{
			ID:    session.ID,
			Slug:  session.Slug,
			State: acp.StateRunning,
		},
		sendErr: errors.New("steer failed"),
	}
	secondID := queuedMessageID(t, store, session.ID, "second")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","id":"`+secondID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded := waitForSession(t, store, session.ID, func(loaded storage.Session) bool {
		return queuedTexts(loaded.QueuedMessages) == "child result|first|second|third" &&
			loaded.Status == storage.StatusError &&
			strings.Contains(loaded.Error, "steer failed")
	})
	if !loaded.QueuedMessages[0].IsInternal() || queuedTexts(loaded.QueuedMessages) != "child result|first|second|third" {
		t.Fatalf("queue = %#v, want internal prefix plus restored public order", loaded.QueuedMessages)
	}
	if loaded.PendingSteer != nil {
		t.Fatalf("pending steer = %#v, want cleared after failed steer", loaded.PendingSteer)
	}
}

func TestRestoreQueuedPromptKeepsPublicPromptAfterNewInternalTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "restore-after-new-internal",
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
	session.QueuedMessages = append([]storage.QueuedMessage{storage.NewInternalQueuedMessage("old child")}, queuedMessages("first", "third")...)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, ACP: &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}}

	if err := srv.StartInternalTurn(context.Background(), session.ID, "new child"); err != nil {
		t.Fatal(err)
	}
	srv.restoreQueuedPrompt(session.ID, storage.NewQueuedMessage("second", nil), 1, "steer failed")

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "old child|new child|first|second|third" ||
		!loaded.QueuedMessages[0].IsInternal() ||
		!loaded.QueuedMessages[1].IsInternal() {
		t.Fatalf("queue = %#v, want restored public prompt after internal prefix", loaded.QueuedMessages)
	}
	if loaded.Status != storage.StatusError || !strings.Contains(loaded.Error, "steer failed") {
		t.Fatalf("status/error = %q/%q, want steer failure", loaded.Status, loaded.Error)
	}
}

func TestRestoreQueuedPromptKeepsInternalPromptBeforeNewInternalTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "restore-internal-order"})
	if err != nil {
		t.Fatal(err)
	}
	session.QueuedMessages = append([]storage.QueuedMessage{storage.NewInternalQueuedMessage("new child")}, queuedMessages("public prompt")...)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store}

	srv.restoreQueuedPrompt(session.ID, storage.NewInternalQueuedMessage("old child"), 0, "hidden start failed")

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "old child|new child|public prompt" ||
		!loaded.QueuedMessages[0].IsInternal() ||
		!loaded.QueuedMessages[1].IsInternal() {
		t.Fatalf("queue = %#v, want restored internal prompt before newer internal turn", loaded.QueuedMessages)
	}
	if loaded.Status != storage.StatusError || !strings.Contains(loaded.Error, "hidden start failed") {
		t.Fatalf("status/error = %q/%q, want hidden start failure", loaded.Status, loaded.Error)
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
