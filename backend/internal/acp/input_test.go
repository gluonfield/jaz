package acp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi/agentsessions"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestManagerInputStartsFreshAgentAfterVoiceConversation(t *testing.T) {
	for _, agent := range []string{acp.AgentGrok, acp.AgentClaude, acp.AgentCodex} {
		t.Run(agent, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			cwd := t.TempDir()
			session, err := store.CreateSession(storage.CreateSession{
				Slug: "voice-first", Runtime: storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: agent, Cwd: cwd},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
				SessionID: session.ID, Type: sessionevents.TypeVoiceMessage,
				Voice: &sessionevents.VoiceMessage{ID: "speech", CallID: "call", Role: "user", Text: "Check disk space"},
			}); err != nil {
				t.Fatal(err)
			}
			manager := newFakeNamedAgentManagerWithOptions(t, store, cwd, agent, map[string]string{
				"JAZ_FAKE_ACP_EXPECT_PROMPT_CONTAINS": "User: Check disk space",
			}, "", "")
			t.Cleanup(manager.Close)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /v1/sessions/{session}/agent/input", agentsessions.NewHandler(manager).Input)
			request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/agent/input", strings.NewReader(`{"message":"Check how much space my laptop has","contexts":[{"type":"voice","id":"call","request_id":"request","text":"User: Check disk space"}]}`))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("voice input HTTP %d: %s", response.Code, response.Body.String())
			}
			job, err := manager.Wait(t.Context(), acp.WaitRequest{Session: session.ID, Timeout: 10 * time.Second})
			if err != nil || job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
				t.Fatalf("first agent result = %+v, %v", job, err)
			}
			stored, err := store.LoadSession(session.ID)
			if err != nil || stored.RuntimeRef.SessionID == "" || len(stored.QueuedMessages) != 0 {
				t.Fatalf("first agent session = %+v, %v", stored, err)
			}
			messages, err := store.LoadMessageRecords(session.ID)
			if err != nil || len(messages) != 1 {
				t.Fatalf("agent messages = %+v, %v", messages, err)
			}
			message := messages[0]
			if message.Content != "Check how much space my laptop has" || len(message.Blocks) != 2 {
				t.Fatalf("stored user message = %+v", message)
			}
			want := storage.Block{Type: storage.BlockTypeVoiceContext, ID: "call", RequestID: "request", Text: "User: Check disk space"}
			block := message.Blocks[0]
			if block.Type != want.Type || block.ID != want.ID || block.RequestID != want.RequestID || block.Text != want.Text {
				t.Fatalf("stored voice context = %+v, want %+v", block, want)
			}
		})
	}
}

func TestManagerInputRejectsUnsupportedSteeringWithoutQueueOrCancel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), nil)
	t.Cleanup(manager.Close)
	spawned, err := manager.Spawn(t.Context(), acp.SpawnRequest{ACPAgent: "fake", Slug: "unsupported"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = manager.Cancel(context.Background(), spawned.SessionID)
	})
	if _, err := manager.Input(t.Context(), acp.SteerRequest{Session: spawned.SessionID, Message: "block until cancelled"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Input(t.Context(), acp.SteerRequest{Session: spawned.SessionID, Message: "follow-up"}); !errors.Is(err, acp.ErrSteeringUnsupported) {
		t.Fatalf("follow-up error = %v", err)
	}
	stored, err := store.LoadSession(spawned.SessionID)
	if err != nil || stored.Status != storage.StatusRunning || len(stored.QueuedMessages) != 0 {
		t.Fatalf("unsupported steering changed active work = %+v, %v", stored, err)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil || len(messages) != 1 {
		t.Fatalf("unaccepted follow-up persisted = %+v, %v", messages, err)
	}
}

func TestManagerAdmissionWaitsForCompletionCallback(t *testing.T) {
	for _, input := range []bool{false, true} {
		name := "send"
		if input {
			name = "input"
		}
		t.Run(name, func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			manager := newFakeAgentManager(t, store, t.TempDir(), nil)
			t.Cleanup(manager.Close)
			entered, release := make(chan struct{}), make(chan struct{})
			var complete, released sync.Once
			t.Cleanup(func() {
				released.Do(func() { close(release) })
			})
			manager.TurnFinished = func(_ context.Context, _ acp.Job) {
				complete.Do(func() {
					close(entered)
					<-release
				})
			}
			spawned, err := manager.Spawn(t.Context(), acp.SpawnRequest{ACPAgent: "fake", Slug: "completion-order"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Input(t.Context(), acp.SteerRequest{Session: spawned.SessionID, Message: "first"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("first turn did not reach its completion callback")
			}
			send := func(ctx context.Context) error {
				if input {
					_, err := manager.Input(ctx, acp.SteerRequest{Session: spawned.SessionID, Message: "next"})
					return err
				}
				_, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "next"})
				return err
			}
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			if err := send(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("next turn admitted before completion persistence finished: %v", err)
			}
			messages, err := store.LoadMessages(spawned.SessionID)
			if err != nil || len(messages) != 1 {
				t.Fatalf("cancelled admission persisted = %+v, %v", messages, err)
			}
			released.Do(func() { close(release) })
			if err := send(t.Context()); err != nil {
				t.Fatal(err)
			}
			job, err := manager.Wait(t.Context(), acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
			if err != nil || job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
				t.Fatalf("next turn result = %+v, %v", job, err)
			}
		})
	}
}
