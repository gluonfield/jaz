package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

type blockingProvider struct {
	started     chan struct{}
	startedOnce sync.Once
	release     chan struct{}
}

func (p *blockingProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *blockingProvider) StreamComplete(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 2)
	go func() {
		defer close(ch)
		p.startedOnce.Do(func() { close(p.started) })
		select {
		case <-p.release:
			ch <- provider.Event{Type: provider.EventDelta, Delta: "done"}
			ch <- provider.Event{Type: provider.EventDone}
		case <-ctx.Done():
			ch <- provider.Event{Type: provider.EventError, Err: ctx.Err()}
		}
	}()
	return ch, nil
}

func TestSessionQueueActionReplacesQueuedMessages(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"messages":[" one ","","two"]}`))
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
	if strings.Join(got.QueuedMessages, "|") != "one|two" {
		t.Fatalf("response queue = %#v", got.QueuedMessages)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(loaded.QueuedMessages, "|") != "one|two" {
		t.Fatalf("stored queue = %#v", loaded.QueuedMessages)
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
	session.QueuedMessages = []string{"first", "second"}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"edit","index":1,"expected":"second","text":"changed"}`))
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
	if strings.Join(loaded.QueuedMessages, "|") != "first|changed" {
		t.Fatalf("queue = %#v", loaded.QueuedMessages)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"delete","index":1,"expected":"second"}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code == http.StatusOK {
		t.Fatal("expected stale expected text to be rejected")
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(loaded.QueuedMessages, "|") != "first|changed" {
		t.Fatalf("stale mutation changed queue: %#v", loaded.QueuedMessages)
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
	session.QueuedMessages = []string{"first"}
	storage.MarkSessionAttention(&session, oldAttention)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"edit","index":0,"expected":"first","text":"changed"}`))
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

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"append","text":"new prompt"}`))
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

func TestSessionQueueActionDoesNotWaitForRunningNativeTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue-running", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	provider := &blockingProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{
			Provider: provider,
			MaxTurns: 1,
		},
	}

	streamReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"keep running"}`))
	streamReq.Header.Set("Content-Type", "application/json")
	streamRes := httptest.NewRecorder()
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		srv.Handler().ServeHTTP(streamRes, streamReq)
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("native provider did not start")
	}

	queueReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"messages":["queued while running"]}`))
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
		close(provider.release)
		t.Fatal("queue action waited for the running native turn")
	}
	if queueRes.Code != http.StatusOK {
		close(provider.release)
		t.Fatalf("queue status = %d, body = %s", queueRes.Code, queueRes.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		close(provider.release)
		t.Fatal(err)
	}
	if strings.Join(loaded.QueuedMessages, "|") != "queued while running" {
		close(provider.release)
		t.Fatalf("queue = %#v", loaded.QueuedMessages)
	}

	close(provider.release)
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("native stream did not finish after release")
	}
	waitForNativeQueueIdle(t, store, session.ID)
}

func waitForNativeQueueIdle(t *testing.T, store storage.SessionStore, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		session, err := store.LoadSession(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if session.Status == storage.StatusIdle && len(session.QueuedMessages) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	session, err := store.LoadSession(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("native queue did not drain: status=%s queued=%#v", session.Status, session.QueuedMessages)
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
	session.QueuedMessages = []string{"next prompt"}
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
	if sent.Completion != acp.CompletionAsync || !sent.Interactive {
		t.Fatalf("queued send should be async interactive: %#v", sent)
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
	session.QueuedMessages = []string{"first", "second"}
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
	if strings.Join(loaded.QueuedMessages, "|") != "second" {
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
	session.QueuedMessages = []string{"first", "second", "third"}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","index":1,"expected":"second"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	answer := waitForACPAnswer(t, manager, "second")
	sent := sentACPRequest(manager)
	if answer.Session != session.ID || answer.Text != "second" {
		t.Fatalf("unexpected interactive answer %#v", answer)
	}
	if sent.Message != "" {
		t.Fatalf("steer should not use queue drain send directly: %#v", sent)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(loaded.QueuedMessages, "|") != "first|third" {
		t.Fatalf("queue = %#v, want selected prompt removed only", loaded.QueuedMessages)
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
	session.QueuedMessages = []string{"steer even after request cancel"}
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
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","index":0}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	_ = waitForACPAnswer(t, manager, "steer even after request cancel")
	manager.mu.Lock()
	answerCtxErr := manager.answerCtxErr
	manager.mu.Unlock()
	if answerCtxErr != nil {
		t.Fatalf("answer used cancelled request context: %v", answerCtxErr)
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
	session.QueuedMessages = []string{"start this now"}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","index":0}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	sent := waitForACPSend(t, manager, "start this now")
	if sent.Session != session.ID || sent.Completion != acp.CompletionAsync || !sent.Interactive {
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
	session.QueuedMessages = []string{"first", "second", "third"}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{
		job: acp.Job{
			ID:    session.ID,
			Slug:  session.Slug,
			State: acp.StateRunning,
		},
		answerErr: errors.New("steer failed"),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", strings.NewReader(`{"op":"steer","index":1,"expected":"second"}`))
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
		return strings.Join(loaded.QueuedMessages, "|") == "first|second|third" &&
			loaded.Status == storage.StatusError &&
			strings.Contains(loaded.Error, "steer failed")
	})
	if strings.Join(loaded.QueuedMessages, "|") != "first|second|third" {
		t.Fatalf("queue = %#v, want failed steer restored at original index", loaded.QueuedMessages)
	}
	if loaded.Status != storage.StatusError || !strings.Contains(loaded.Error, "steer failed") {
		t.Fatalf("status/error = %q/%q", loaded.Status, loaded.Error)
	}
}

func waitForACPAnswer(t *testing.T, manager *fakeACPManager, text string) acp.InteractiveAnswer {
	t.Helper()
	var answer acp.InteractiveAnswer
	waitFor(t, time.Second, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		answer = manager.answered
		return answer.Text == text
	})
	return answer
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

func TestQueuedNativeDrainRunsPromptWithoutClientStream(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-queue", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	session.QueuedMessages = []string{"native queued prompt"}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{
			Provider: mockprovider.New(),
			MaxTurns: 4,
		},
	}

	srv.drainQueuedPrompt(context.Background(), session.ID)

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusIdle {
		t.Fatalf("status = %q, want idle", loaded.Status)
	}
	if len(loaded.QueuedMessages) != 0 {
		t.Fatalf("queue was not drained: %#v", loaded.QueuedMessages)
	}
	messages, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) == 0 || provider.MessageContent(messages[0]) != "native queued prompt" {
		t.Fatalf("missing queued user prompt: %#v", messages)
	}
	if provider.MessageContent(messages[len(messages)-1]) != "Mock provider received the tool result and finished." {
		t.Fatalf("missing assistant completion: %#v", messages[len(messages)-1])
	}
}
