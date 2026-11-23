package cloudsync_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestProjectTranscriptAndStreamProjectorRichEvents(t *testing.T) {
	now := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	thread := storage.CloudThread{
		ID:     "ct_rich",
		Status: "idle",
	}
	rawEvents := []storage.CloudThreadEvent{
		{
			ThreadID:    "ct_rich",
			Seq:         1,
			ActorUserID: "u1",
			Type:        "user_message",
			PayloadJSON: json.RawMessage(`{"text":"Run go test"}`),
			CreatedAt:   now,
		},
		{
			ThreadID:    "ct_rich",
			Seq:         2,
			Type:        "thinking_delta",
			PayloadJSON: json.RawMessage(`{"delta":"Let me "}`),
			CreatedAt:   now.Add(time.Second),
		},
		{
			ThreadID:    "ct_rich",
			Seq:         3,
			Type:        "thinking_delta",
			PayloadJSON: json.RawMessage(`{"delta":"run tests."}`),
			CreatedAt:   now.Add(2 * time.Second),
		},
		{
			ThreadID:    "ct_rich",
			Seq:         4,
			Type:        "tool_call",
			PayloadJSON: json.RawMessage(`{"call_id":"call_1","tool":"commandExecution","title":"go test ./...","status":"running","arguments":{"command":"go test ./..."}}`),
			CreatedAt:   now.Add(3 * time.Second),
		},
		{
			ThreadID:    "ct_rich",
			Seq:         5,
			Type:        "tool_delta",
			PayloadJSON: json.RawMessage(`{"call_id":"call_1","delta":"PASS\n"}`),
			CreatedAt:   now.Add(4 * time.Second),
		},
		{
			ThreadID:    "ct_rich",
			Seq:         6,
			Type:        "tool_result",
			PayloadJSON: json.RawMessage(`{"call_id":"call_1","tool":"commandExecution","status":"completed","output":"PASS\nok pkg 0.1s","error":""}`),
			CreatedAt:   now.Add(5 * time.Second),
		},
		{
			ThreadID:    "ct_rich",
			Seq:         7,
			Type:        "assistant_delta",
			PayloadJSON: json.RawMessage(`{"delta":"All tests "}`),
			CreatedAt:   now.Add(6 * time.Second),
		},
		{
			ThreadID:    "ct_rich",
			Seq:         8,
			Type:        "assistant_delta",
			PayloadJSON: json.RawMessage(`{"delta":"passed."}`),
			CreatedAt:   now.Add(7 * time.Second),
		},
		{
			ThreadID:    "ct_rich",
			Seq:         9,
			Type:        "turn_completed",
			PayloadJSON: json.RawMessage(`{"status":"completed"}`),
			CreatedAt:   now.Add(8 * time.Second),
		},
	}

	transcript := cloudsync.ProjectTranscript(thread, rawEvents)
	if len(transcript.Messages) != 2 {
		t.Fatalf("expected 2 projected messages, got %d: %+v", len(transcript.Messages), transcript.Messages)
	}
	assistant := transcript.Messages[1]
	if assistant.Content != "All tests passed." {
		t.Fatalf("assistant content = %q, want %q", assistant.Content, "All tests passed.")
	}
	if assistant.Reasoning != "Let me run tests." {
		t.Fatalf("assistant reasoning = %q, want %q", assistant.Reasoning, "Let me run tests.")
	}
	if len(assistant.Blocks) != 3 {
		t.Fatalf("expected 3 ordered blocks (reasoning, tool, text), got %d: %+v", len(assistant.Blocks), assistant.Blocks)
	}
	if assistant.Blocks[0].Type != storage.BlockTypeReasoning || assistant.Blocks[0].Text != "Let me run tests." {
		t.Fatalf("unexpected block[0]: %+v", assistant.Blocks[0])
	}
	if assistant.Blocks[1].Type != storage.BlockTypeTool || assistant.Blocks[1].ID != "call_1" || assistant.Blocks[1].Name != "commandExecution" || assistant.Blocks[1].Result != "PASS\nok pkg 0.1s" {
		t.Fatalf("unexpected block[1]: %+v", assistant.Blocks[1])
	}
	if assistant.Blocks[2].Type != storage.BlockTypeText || assistant.Blocks[2].Text != "All tests passed." {
		t.Fatalf("unexpected block[2]: %+v", assistant.Blocks[2])
	}

	var foundThought, foundTool bool
	for _, ev := range transcript.Events {
		if ev.Type == sessionevents.TypeACPThought && ev.ACP != nil && ev.ACP.Thought == "Let me run tests." {
			foundThought = true
		}
		if ev.Type == "acp_tool" && ev.ACP != nil && len(ev.ACP.ToolCalls) == 1 {
			tc := ev.ACP.ToolCalls[0]
			if tc.ID == "call_1" && tc.Status == "completed" && tc.Title == "go test ./..." && tc.Runtime.TerminalOutput == "PASS\nok pkg 0.1s" {
				foundTool = true
			}
		}
	}
	if !foundThought || !foundTool {
		t.Fatalf("expected compacted acp_thought and acp_tool events in transcript.Events, got %+v", transcript.Events)
	}

	// Verify live StreamProjector emits incremental acp_thought + coalesced acp_tool updates.
	sp := cloudsync.NewStreamProjector("ct_rich")
	evThought1 := sp.Convert(rawEvents[1])
	evThought2 := sp.Convert(rawEvents[2])
	if evThought1.Type != sessionevents.TypeACPThought || evThought1.ACP == nil || evThought1.ACP.Thought != "Let me " {
		t.Fatalf("unexpected first thought stream event: %+v", evThought1)
	}
	if evThought2.Type != sessionevents.TypeACPThought || evThought2.ProjectionKey != evThought1.ProjectionKey || evThought2.ProjectionOp != sessionevents.ProjectionAppend || evThought2.ACP.Thought != "run tests." {
		t.Fatalf("unexpected second thought stream event: %+v", evThought2)
	}
	evCall := sp.Convert(rawEvents[3])
	evDelta := sp.Convert(rawEvents[4])
	evResult := sp.Convert(rawEvents[5])
	if evCall.Type != "acp_tool" || evCall.ProjectionKey != "acp_tool:ct_rich:call_1" || evCall.ACP.ToolCalls[0].Status != "in_progress" {
		t.Fatalf("unexpected tool_call stream event: %+v", evCall)
	}
	if evDelta.Type != "acp_tool" || evDelta.ProjectionKey != evCall.ProjectionKey || evDelta.ACP.ToolCalls[0].Runtime.TerminalOutput != "PASS\n" {
		t.Fatalf("unexpected tool_delta stream event: %+v", evDelta)
	}
	if evResult.Type != "acp_tool" || evResult.ACP.ToolCalls[0].Status != "completed" || evResult.ACP.ToolCalls[0].Runtime.TerminalOutput != "PASS\nok pkg 0.1s" {
		t.Fatalf("unexpected tool_result stream event: %+v", evResult)
	}
}

func TestCancelAndSteerThreadRequests(t *testing.T) {
	ctx := context.Background()
	var (
		mu          sync.Mutex
		cancelCalls int
		steerBodies []map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
		case r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": "org1", "email": "u@cas.dev", "display_name": "Dev", "role": "member"})
		case r.URL.Path == "/v1/spaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
		case r.URL.Path == "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{
					{
						"id": "th_1", "org_id": "org1", "owner_user_id": "u1", "run_id": "th_1",
						"visibility": "private", "title": "Active", "status": "running",
						"updated_at": time.Now().UTC().Format(time.RFC3339),
					},
				},
			})
		case r.URL.Path == "/v1/usage/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
		case r.URL.Path == "/v1/threads/th_1/cancel" && r.Method == http.MethodPost:
			mu.Lock()
			cancelCalls++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"canceled": true, "thread_id": "th_1"})
		case r.URL.Path == "/v1/threads/th_1/steer" && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			steerBodies = append(steerBodies, body)
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"mode": body["mode"], "action": "queued", "queued": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	if _, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srv.URL, Token: "tok-1"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if _, _, err := svc.CancelThread(ctx, "../evil"); !errors.Is(err, cloudsync.ErrInvalidPath) {
		t.Fatalf("CancelThread invalid path err = %v, want ErrInvalidPath", err)
	}
	if _, _, err := svc.SteerThread(ctx, "../evil", "hi", "steer"); !errors.Is(err, cloudsync.ErrInvalidPath) {
		t.Fatalf("SteerThread invalid path err = %v, want ErrInvalidPath", err)
	}

	raw, code, err := svc.CancelThread(ctx, "th_1")
	if err != nil || code != http.StatusOK || len(raw) == 0 {
		t.Fatalf("CancelThread failed: code=%d raw=%s err=%v", code, string(raw), err)
	}
	sess, found, err := svc.GetSession(ctx, "th_1")
	if err != nil || !found || sess.Status != storage.StatusIdle {
		t.Fatalf("expected mirrored thread status idle after cancel, got found=%v status=%q err=%v", found, sess.Status, err)
	}

	rawSteer, codeSteer, err := svc.SteerThread(ctx, "th_1", "switch to plan B", "queue")
	if err != nil || codeSteer != http.StatusAccepted || len(rawSteer) == 0 {
		t.Fatalf("SteerThread failed: code=%d raw=%s err=%v", codeSteer, string(rawSteer), err)
	}
	sess, found, err = svc.GetSession(ctx, "th_1")
	if err != nil || !found || sess.Status != storage.StatusRunning {
		t.Fatalf("expected mirrored thread status running after steer, got found=%v status=%q err=%v", found, sess.Status, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if cancelCalls != 1 {
		t.Fatalf("expected 1 cancel call, got %d", cancelCalls)
	}
	if len(steerBodies) != 1 || steerBodies[0]["text"] != "switch to plan B" || steerBodies[0]["mode"] != "queue" {
		t.Fatalf("unexpected steer payload: %+v", steerBodies)
	}
}
