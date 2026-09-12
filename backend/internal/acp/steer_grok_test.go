package acp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi/agentsessions"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestVoiceInputUsesGrokInterjectionsUntilNativeCompletion(t *testing.T) {
	for _, test := range []struct{ name, late, stop string }{
		{"active", "0", ""},
		{"fallback", "1", ""},
		{"fallback completes before disconnect", "1", "complete-disconnect"},
		{"fallback disconnect", "1", "disconnect"},
		{"fallback shutdown", "1", "shutdown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			dir := t.TempDir()
			requestLog := filepath.Join(dir, "requests.jsonl")
			release := filepath.Join(dir, "release")
			manager := newFakeAgentManager(t, store, dir, map[string]string{
				"JAZ_FAKE_ACP_GROK_INTERJECT": "1",
				"JAZ_FAKE_ACP_GROK_LATE":      test.late,
				"JAZ_FAKE_ACP_GROK_STOP":      test.stop,
				"JAZ_FAKE_ACP_GROK_RELEASE":   release,
				"JAZ_FAKE_ACP_REQUEST_LOG":    requestLog,
			})
			t.Cleanup(manager.Close)
			finished := make(chan acp.Job, 4)
			manager.TurnFinished = func(_ context.Context, result acp.Job) { finished <- result }
			spawned, err := manager.Spawn(t.Context(), acp.SpawnRequest{ACPAgent: "fake", Slug: "grok-steering"})
			if err != nil {
				t.Fatal(err)
			}
			input := func(text, id string) {
				t.Helper()
				body, _ := json.Marshal(map[string]any{"message": text, "contexts": []storage.MessageContext{{Type: storage.ContextTypeVoice, ID: "call", RequestID: id, Text: "User: " + text}}})
				request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
				request.SetPathValue("session", spawned.SessionID)
				response := httptest.NewRecorder()
				agentsessions.NewHandler(manager).Input(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("voice input: HTTP %d %s", response.Code, response.Body.String())
				}
			}
			input("block until cancelled", "initial")
			input("Find 30 references.", "first")
			input("Use only the last six months.", "second")
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(release + ".acknowledged"); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("Grok did not acknowledge the interjections")
				}
				time.Sleep(time.Millisecond)
			}
			job, err := manager.Status(spawned.SessionID)
			if err != nil || job.State != acp.StateRunning {
				t.Fatalf("acknowledgement completed work prematurely: %+v, %v", job, err)
			}
			if test.stop == "shutdown" {
				manager.Close()
				job, err = manager.Status(spawned.SessionID)
				if err != nil || job.State != acp.StateCancelled || job.StopReason != acp.StopReasonServerShutdown {
					t.Fatalf("shutdown left native work running: %+v, %v", job, err)
				}
				return
			}
			if err := os.WriteFile(release, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			job, err = manager.Wait(t.Context(), acp.WaitRequest{Session: spawned.SessionID, Timeout: 5 * time.Second})
			if test.stop == "complete-disconnect" {
				job = <-finished
			}
			if test.stop == "disconnect" {
				if err != nil || job.State != acp.StateFailed || job.Error == "" {
					t.Fatalf("lost Grok connection left native work running: %+v, %v", job, err)
				}
				return
			}
			if err != nil || job.State != acp.StateIdle || job.Assistant != "Both follow-ups received." {
				t.Fatalf("native completion: %+v, %v", job, err)
			}
			requests, _ := os.ReadFile(requestLog)
			wire := string(requests)
			if strings.Count(wire, `"method":"session/prompt"`) != 1 || strings.Count(wire, `"method":"_x.ai/interject"`) != 2 || strings.Contains(wire, `"method":"session/cancel"`) {
				t.Fatalf("unexpected native requests: %s", wire)
			}
			if !strings.Contains(wire, "User: Use only the last six months.") {
				t.Fatal("voice context was lost")
			}
			session, err := store.LoadSession(spawned.SessionID)
			if err != nil || len(session.QueuedMessages) != 0 {
				t.Fatalf("voice entered the chat queue: %+v, %v", session, err)
			}
			wantInput := int64(100)
			if test.late == "1" {
				wantInput += 20
			}
			if session.Usage.InputTokens != wantInput {
				t.Fatalf("native usage lost or counted twice: %+v", session.Usage)
			}
			messages, err := store.LoadMessageRecords(spawned.SessionID)
			if err != nil || len(messages) != 3 || messages[2].Content != "Use only the last six months." {
				t.Fatalf("voice requests not persisted: %+v, %v", messages, err)
			}
		})
	}
}

func fakeGrokInterject(t *testing.T, conn jsonrpc.MessageConn, message, pending *jsonrpc.Message, count int) *jsonrpc.Message {
	t.Helper()
	var request struct {
		SessionID string            `json:"sessionId"`
		ID        string            `json:"interjectionId"`
		Text      string            `json:"text"`
		Content   []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(message.Params, &request); err != nil || request.ID == "" || request.Text == "" || len(request.Content) == 0 {
		t.Fatal("invalid Grok interjection request")
	}
	late := os.Getenv("JAZ_FAKE_ACP_GROK_LATE") == "1"
	if late && pending != nil {
		notify(conn, "_x.ai/session_notification", map[string]any{"sessionId": request.SessionID, "update": map[string]any{"sessionUpdate": "turn_completed", "prompt_id": "original", "stop_reason": "end_turn"}})
		sendResult(conn, pending, map[string]any{"stopReason": "end_turn", "_meta": map[string]any{"promptId": "original", "usage": map[string]any{"inputTokens": 100, "outputTokens": 10, "totalTokens": 110}}})
		pending = nil
	}
	sendResult(conn, message, map[string]any{"result": map[string]any{"status": "queued"}})
	notify(conn, "_x.ai/session/interjection", map[string]any{"sessionId": request.SessionID, "interjectionId": request.ID, "text": request.Text})
	if count < 2 {
		return pending
	}
	release := os.Getenv("JAZ_FAKE_ACP_GROK_RELEASE")
	if err := os.WriteFile(release+".acknowledged", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			if _, err := os.Stat(release); err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if os.Getenv("JAZ_FAKE_ACP_GROK_STOP") == "disconnect" {
			os.Exit(0)
		}
		notify(conn, "session/update", map[string]any{"sessionId": request.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Both follow-ups received."}}})
		promptID := "original"
		inputTokens := 100
		if late {
			promptID = "interject-fallback-native-id"
			inputTokens = 20
		}
		completed := map[string]any{"sessionId": request.SessionID, "update": map[string]any{"sessionUpdate": "turn_completed", "prompt_id": promptID, "stop_reason": "end_turn", "usage": map[string]any{"inputTokens": inputTokens, "outputTokens": 10, "totalTokens": inputTokens + 10}}}
		notify(conn, "_x.ai/session_notification", completed)
		notify(conn, "_x.ai/session_notification", completed)
		if pending != nil {
			sendResult(conn, pending, map[string]any{"stopReason": "end_turn", "_meta": map[string]any{"promptId": "original", "usage": map[string]any{"inputTokens": 100, "outputTokens": 10, "totalTokens": 110}}})
		}
		if os.Getenv("JAZ_FAKE_ACP_GROK_STOP") == "complete-disconnect" {
			os.Exit(0)
		}
	}()
	return nil
}
