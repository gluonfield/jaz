package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionQueueActionAppendsQueuedMessage(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"append","message":{"text":" one "}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got storage.Session
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if queuedTexts(got.QueuedMessages) != "one" {
		t.Fatalf("response queue = %#v", got.QueuedMessages)
	}
	if got.QueuedMessages[0].ID == "" {
		t.Fatalf("response queue missing id: %#v", got.QueuedMessages)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "one" {
		t.Fatalf("stored queue = %#v", loaded.QueuedMessages)
	}
	if loaded.QueuedMessages[0].ID == "" {
		t.Fatalf("stored queue missing id: %#v", loaded.QueuedMessages)
	}
}

func TestSessionQueueActionStoresJazGoalRequest(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "queue-goal-unsupported",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: acp.AgentClaude,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"append","message":{"text":"keep going","goal_requested":true}}`))
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
	if len(loaded.QueuedMessages) != 1 || !loaded.QueuedMessages[0].GoalRequested {
		t.Fatalf("queue = %#v, want one goal-requested prompt", loaded.QueuedMessages)
	}
}

func TestSessionQueueActionMutatesStoredQueue(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue-mutate"})
	if err != nil {
		t.Fatal(err)
	}
	session.QueuedMessages = queuedMessages("first", "second")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	secondID := queuedMessageID(t, store, session.ID, "second")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"edit","id":"`+secondID+`","message":{"text":"changed"}}`))
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
	if queuedTexts(loaded.QueuedMessages) != "first|changed" {
		t.Fatalf("queue = %#v", loaded.QueuedMessages)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"delete","id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code == http.StatusOK {
		t.Fatal("expected stale queued prompt id to be rejected")
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "first|changed" {
		t.Fatalf("stale mutation changed queue: %#v", loaded.QueuedMessages)
	}
}

func TestSessionQueueActionReordersQueuedMessagesByID(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue-reorder"})
	if err != nil {
		t.Fatal(err)
	}
	session.QueuedMessages = queuedMessages("first", "second", "third")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	firstID := queuedMessageID(t, store, session.ID, "first")
	secondID := queuedMessageID(t, store, session.ID, "second")
	thirdID := queuedMessageID(t, store, session.ID, "third")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"reorder","ids":["`+thirdID+`","`+firstID+`","`+secondID+`"]}`))
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
	if queuedTexts(loaded.QueuedMessages) != "third|first|second" {
		t.Fatalf("queue = %#v", loaded.QueuedMessages)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"reorder","ids":["`+thirdID+`","`+thirdID+`","`+firstID+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	(&Server{Store: store}).Handler().ServeHTTP(res, req)
	if res.Code == http.StatusOK {
		t.Fatal("expected duplicate queued prompt ids to be rejected")
	}
}

func TestSessionQueueActionAppendsAttachmentIDs(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue-attachment"})
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, Workspace: t.TempDir()}
	handler := srv.Handler()
	attachment := uploadTestAttachment(t, handler, session.ID, "image.png", "image-bytes")

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"append","message":{"text":"inspect this","attachment_ids":["`+attachment.ID+`"]}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "inspect this" {
		t.Fatalf("queue = %#v", loaded.QueuedMessages)
	}
	if got := loaded.QueuedMessages[0].AttachmentIDs; len(got) != 1 || got[0] != attachment.ID {
		t.Fatalf("queued attachment ids = %#v", got)
	}
}

func TestSessionQueueAttentionFollowsUserSendNotQueueEdits(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue-attention"})
	if err != nil {
		t.Fatal(err)
	}
	oldAttention := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	session.QueuedMessages = queuedMessages("first")
	storage.MarkSessionAttention(&session, oldAttention)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	firstID := queuedMessageID(t, store, session.ID, "first")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"edit","id":"`+firstID+`","message":{"text":"changed"}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	(&Server{Store: store}).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("edit status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LastAttentionAt.Equal(oldAttention) {
		t.Fatalf("edit changed attention: %s -> %s", oldAttention, loaded.LastAttentionAt)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"append","message":{"text":"new prompt"}}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	(&Server{Store: store}).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("append status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LastAttentionAt.After(oldAttention) {
		t.Fatalf("append did not advance attention: %s -> %s", oldAttention, loaded.LastAttentionAt)
	}
}

func TestSessionQueueActionDoesNotWaitForRunningACPTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "queue-running",
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
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, ACP: &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}}

	queueReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"append","message":{"text":"queued while running"}}`))
	queueReq.Header.Set("Content-Type", "application/json")
	queueRes := httptest.NewRecorder()
	queueDone := make(chan struct{})
	go func() {
		defer close(queueDone)
		srv.Handler().ServeHTTP(queueRes, queueReq)
	}()
	select {
	case <-queueDone:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("queue action waited for the running ACP turn")
	}
	if queueRes.Code != http.StatusOK {
		t.Fatalf("queue status = %d, body = %s", queueRes.Code, queueRes.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "queued while running" {
		t.Fatalf("queue = %#v", loaded.QueuedMessages)
	}
	if loaded.Status != storage.StatusRunning {
		t.Fatalf("status = %q, want running", loaded.Status)
	}
}

func queuedMessages(texts ...string) []storage.QueuedMessage {
	out := make([]storage.QueuedMessage, 0, len(texts))
	for i, text := range texts {
		message := storage.NewQueuedMessage(text, nil)
		message.ID = fmt.Sprintf("queue-test-%d", i)
		out = append(out, message)
	}
	return out
}

func queuedMessageID(t *testing.T, store storage.SessionStore, sessionID string, text string) string {
	t.Helper()
	session, err := store.LoadSession(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range session.QueuedMessages {
		if message.Text == text {
			if message.ID == "" {
				t.Fatalf("queued message %q has no id: %#v", text, session.QueuedMessages)
			}
			return message.ID
		}
	}
	t.Fatalf("queued message %q not found in %#v", text, session.QueuedMessages)
	return ""
}

func queuedTexts(messages []storage.QueuedMessage) string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		if text := strings.TrimSpace(message.Text); text != "" {
			out = append(out, text)
		}
	}
	return strings.Join(out, "|")
}

func TestACPTurnFinishedDrainsQueuedPromptWithoutActiveClient(t *testing.T) {
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
	session.QueuedMessages = queuedMessages("next prompt")
	session.QueuedMessages[0].PlanRequested = true
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	}}
	srv := &Server{Store: store, ACP: manager}

	srv.HandleACPTurnFinished(context.Background(), acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	})

	var sent acp.SendRequest
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		sent = manager.sent
		manager.mu.Unlock()
		if sent.Message != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sent.Session != session.ID || sent.Message != "next prompt" {
		t.Fatalf("unexpected queued send %#v", sent)
	}
	if sent.Completion != acp.CompletionAsync {
		t.Fatalf("queued send should be async: %#v", sent)
	}
	if !sent.PlanRequested {
		t.Fatalf("queued send did not preserve plan mode: %#v", sent)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 0 {
		t.Fatalf("queue was not popped: %#v", loaded.QueuedMessages)
	}
	if loaded.Status != storage.StatusRunning {
		t.Fatalf("status = %q, want running while queued prompt is active", loaded.Status)
	}
}

func TestQueuedACPDrainSendsAttachments(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue-attachment",
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
	workspace := t.TempDir()
	srv := &Server{Store: store, Workspace: workspace}
	attachment := uploadTestAttachment(t, srv.Handler(), session.ID, "image.png", "image-bytes")
	session.QueuedMessages = []storage.QueuedMessage{
		storage.NewQueuedMessage("inspect this", []string{attachment.ID}),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	}}
	srv.ACP = manager

	srv.drainQueuedPrompt(context.Background(), session.ID)

	sent := waitForACPSend(t, manager, "inspect this")
	if len(sent.Attachments) != 1 {
		t.Fatalf("attachments = %#v", sent.Attachments)
	}
	if got := sent.Attachments[0]; got.ID != attachment.ID || got.URI != "" || got.ServerPath != attachment.ServerPath {
		t.Fatalf("sent attachment = %#v, want %#v", got, attachment)
	}
}

func TestQueuedACPDrainClaimsOnlyOnePrompt(t *testing.T) {
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
	session.QueuedMessages = queuedMessages("first", "second")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	}}
	srv := &Server{Store: store, ACP: manager}

	srv.drainQueuedPrompt(context.Background(), session.ID)

	manager.mu.Lock()
	sent := manager.sent
	manager.mu.Unlock()
	if sent.Session != session.ID || sent.Message != "first" {
		t.Fatalf("unexpected queued send %#v", sent)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "second" {
		t.Fatalf("queue = %#v, want only second prompt left", loaded.QueuedMessages)
	}
}

func TestSessionQueueSteerClaimsOnePromptForRunningACP(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue-steer",
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
	session.QueuedMessages = queuedMessages("first", "second", "third")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}

	secondID := queuedMessageID(t, store, session.ID, "second")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","id":"`+secondID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	sent := waitForACPSend(t, manager, "second")
	if sent.Session != session.ID || sent.Completion != acp.CompletionAsync {
		t.Fatalf("unexpected steer send %#v", sent)
	}
	manager.mu.Lock()
	answered := manager.answered
	cancelCtxErr := manager.cancelCtxErr
	sendCtxErr := manager.sendCtxErr
	manager.mu.Unlock()
	if answered.Text != "" {
		t.Fatalf("steer should not use text-only interactive answer: %#v", answered)
	}
	if cancelCtxErr != nil || sendCtxErr != nil {
		t.Fatalf("steer used cancelled context: cancel=%v send=%v", cancelCtxErr, sendCtxErr)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedTexts(loaded.QueuedMessages) != "first|third" {
		t.Fatalf("queue = %#v, want selected prompt removed only", loaded.QueuedMessages)
	}
	if loaded.PendingSteer != nil {
		t.Fatalf("pending steer = %#v, want cleared after send", loaded.PendingSteer)
	}
}

func TestSessionQueueSteerUsesPromptQueueingWhenSupported(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "claude-queue-steer",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "claude",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusRunning
	session.QueuedMessages = queuedMessages("follow this")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{
		job: acp.Job{
			ID:    session.ID,
			Slug:  session.Slug,
			State: acp.StateRunning,
		},
		steerSupported: true,
	}

	promptID := queuedMessageID(t, store, session.ID, "follow this")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","id":"`+promptID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	steered := waitForACPSteer(t, manager, "follow this")
	if steered.Session != session.ID {
		t.Fatalf("unexpected steer %#v", steered)
	}
	manager.mu.Lock()
	sent := manager.sent
	cancelled := manager.cancelled
	steerCtxErr := manager.steerCtxErr
	manager.mu.Unlock()
	if sent.Message != "" {
		t.Fatalf("prompt queueing steer should not fall back to send: %#v", sent)
	}
	if cancelled {
		t.Fatal("prompt queueing steer called cancel")
	}
	if steerCtxErr != nil {
		t.Fatalf("steer used cancelled context: %v", steerCtxErr)
	}
	var loaded storage.Session
	waitFor(t, time.Second, func() bool {
		var err error
		loaded, err = store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(loaded.QueuedMessages) == 0 && loaded.PendingSteer == nil
	})
	if len(loaded.QueuedMessages) != 0 || loaded.PendingSteer != nil {
		t.Fatalf("session queue=%#v pending=%#v", loaded.QueuedMessages, loaded.PendingSteer)
	}
}

func TestSessionQueueSteerShowsPendingBeforeRunningACPStops(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue-steer-pending",
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
	session.QueuedMessages = queuedMessages("first", "second")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{
		job: acp.Job{
			ID:    session.ID,
			Slug:  session.Slug,
			State: acp.StateRunning,
		},
		cancelRelease: make(chan struct{}),
	}

	firstID := queuedMessageID(t, store, session.ID, "first")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","id":"`+firstID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PendingSteer == nil || loaded.PendingSteer.Text != "first" {
		t.Fatalf("pending steer = %#v, want first prompt visible", loaded.PendingSteer)
	}
	if queuedTexts(loaded.QueuedMessages) != "second" {
		t.Fatalf("queue = %#v, want selected prompt removed", loaded.QueuedMessages)
	}
	if sent := sentACPRequest(manager); sent.Message != "" {
		t.Fatalf("sent before cancel completed: %#v", sent)
	}
	close(manager.cancelRelease)
	_ = waitForACPSend(t, manager, "first")
	waitFor(t, time.Second, func() bool {
		loaded, err = store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return loaded.PendingSteer == nil
	})
}

func TestSessionQueueSteerSendsAttachmentsForRunningACP(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue-steer-attachment",
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
	workspace := t.TempDir()
	srv := &Server{Store: store, Workspace: workspace}
	attachment := uploadTestAttachment(t, srv.Handler(), session.ID, "image.png", "image-bytes")
	session.Status = storage.StatusRunning
	message := storage.NewQueuedMessage("inspect this", []string{attachment.ID})
	message.ID = "queue-test-0"
	session.QueuedMessages = []storage.QueuedMessage{message}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}
	srv.ACP = manager

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","id":"queue-test-0"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	sent := waitForACPSend(t, manager, "inspect this")
	if len(sent.Attachments) != 1 {
		t.Fatalf("attachments = %#v", sent.Attachments)
	}
	if got := sent.Attachments[0]; got.ID != attachment.ID || got.URI != "" || got.ServerPath != attachment.ServerPath {
		t.Fatalf("sent attachment = %#v, want %#v", got, attachment)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 0 || loaded.Status != storage.StatusRunning {
		t.Fatalf("session after steer queue=%#v status=%q", loaded.QueuedMessages, loaded.Status)
	}
}

func TestSessionQueueSteerUsesServerContextAfterRequestCancel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue-steer-cancelled-request",
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
	session.QueuedMessages = queuedMessages("steer even after request cancel")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	promptID := queuedMessageID(t, store, session.ID, "steer even after request cancel")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","id":"`+promptID+`"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	_ = waitForACPSend(t, manager, "steer even after request cancel")
	manager.mu.Lock()
	cancelCtxErr := manager.cancelCtxErr
	sendCtxErr := manager.sendCtxErr
	manager.mu.Unlock()
	if cancelCtxErr != nil || sendCtxErr != nil {
		t.Fatalf("steer used cancelled request context: cancel=%v send=%v", cancelCtxErr, sendCtxErr)
	}
}

func TestSessionQueueSteerStartsIdleACPFromBackend(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue-steer-idle",
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
	session.QueuedMessages = queuedMessages("start this now")
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	}}

	promptID := queuedMessageID(t, store, session.ID, "start this now")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","id":"`+promptID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	sent := waitForACPSend(t, manager, "start this now")
	if sent.Session != session.ID || sent.Completion != acp.CompletionAsync {
		t.Fatalf("unexpected steer send %#v", sent)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 0 || loaded.Status != storage.StatusRunning {
		t.Fatalf("session after steer queue=%#v status=%q", loaded.QueuedMessages, loaded.Status)
	}
}

func TestSessionQueueSteerRestoresPromptWhenSendFails(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-queue-steer-fail",
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
	session.QueuedMessages = queuedMessages("first", "second", "third")
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
	var loaded storage.Session
	waitFor(t, time.Second, func() bool {
		loaded, err = store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return queuedTexts(loaded.QueuedMessages) == "first|second|third" &&
			loaded.Status == storage.StatusError &&
			strings.Contains(loaded.Error, "steer failed")
	})
	if queuedTexts(loaded.QueuedMessages) != "first|second|third" {
		t.Fatalf("queue = %#v, want failed steer restored at original index", loaded.QueuedMessages)
	}
	if loaded.PendingSteer != nil {
		t.Fatalf("pending steer = %#v, want cleared after failed steer", loaded.PendingSteer)
	}
	if loaded.Status != storage.StatusError || !strings.Contains(loaded.Error, "steer failed") {
		t.Fatalf("status/error = %q/%q", loaded.Status, loaded.Error)
	}
}

func waitForACPSend(t *testing.T, manager *fakeACPManager, text string) acp.SendRequest {
	t.Helper()
	var sent acp.SendRequest
	waitFor(t, time.Second, func() bool {
		sent = sentACPRequest(manager)
		return sent.Message == text
	})
	return sent
}

func waitForACPSteer(t *testing.T, manager *fakeACPManager, text string) acp.SteerRequest {
	t.Helper()
	var steered acp.SteerRequest
	waitFor(t, time.Second, func() bool {
		manager.mu.Lock()
		steered = manager.steered
		manager.mu.Unlock()
		return steered.Message == text
	})
	return steered
}

func sentACPRequest(manager *fakeACPManager) acp.SendRequest {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.sent
}

func waitFor(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready() {
		t.Fatal("condition not met before timeout")
	}
}
