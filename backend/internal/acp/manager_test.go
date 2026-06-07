package acp_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestManagerSpawnsFakeACPAgentAndStoresSession(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:         t.TempDir(),
		Workspace:    t.TempDir(),
		SystemPrompt: "skill prompt",
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1", "JAZ_FAKE_ACP_SYSTEM_PROMPT": "skill prompt"},
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{
		ParentID: parent.ID,
		ACPAgent: "fake",
		Slug:     "fake-review",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	if spawned.State != acp.StateIdle {
		t.Fatalf("spawn state = %s, want %s", spawned.State, acp.StateIdle)
	}
	status, err := manager.Status(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Modes.PlanModeID != "plan" || status.Modes.ExecutionModeID != "full-access" {
		t.Fatalf("unexpected modes %#v", status.Modes)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("spawn should not store task messages: %#v", messages)
	}

	done := make(chan acp.Job, 2)
	manager.Done = func(_ context.Context, job acp.Job) { done <- job }

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "say hello", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle {
		t.Fatalf("state = %s, want %s; error=%s", job.State, acp.StateIdle, job.Error)
	}
	if job.Assistant != "hello from fake agent" {
		t.Fatalf("assistant = %q", job.Assistant)
	}

	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ParentID != parent.ID || session.Runtime != storage.RuntimeACP || session.RuntimeRef.SessionID != "fake-session" {
		t.Fatalf("unexpected session metadata %#v", session)
	}
	messages, err = store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || provider.MessageContent(messages[0]) != "say hello" {
		t.Fatalf("unexpected messages %#v", messages)
	}
	events, err := store.LoadSessionEvents(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasACPMessage(events, "hello from fake agent") || !hasACPTool(events, "whoami") {
		t.Fatalf("missing ACP transcript events %#v", events)
	}
	select {
	case job := <-done:
		t.Fatalf("sync task propagated async completion: %#v", job)
	case <-time.After(100 * time.Millisecond):
	}
	activity, err := store.LoadActivity(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) != 1 || activity[0].Text != "whoami" || activity[0].Status != "completed" {
		t.Fatalf("unexpected activity %#v", activity)
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "again", Completion: acp.CompletionAsync, ParentVisible: true}); err != nil {
		t.Fatal(err)
	}
	job, err = manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.Assistant != "hello from fake agent" {
		t.Fatalf("assistant after follow-up = %q", job.Assistant)
	}
	messages, err = store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || provider.MessageContent(messages[1]) != "again" {
		t.Fatalf("unexpected follow-up messages %#v", messages)
	}
	events, err = store.LoadSessionEvents(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if countACPMessage(events, "hello from fake agent") < 2 {
		t.Fatalf("missing follow-up ACP transcript event %#v", events)
	}
	parentEvents, err := store.LoadSessionEvents(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasACPMessage(parentEvents, "hello from fake agent") || hasACPTool(parentEvents, "whoami") {
		t.Fatalf("parent leaked child transcript details %#v", parentEvents)
	}
	if !hasACPStatus(parentEvents, spawned.SessionID) {
		t.Fatalf("parent missing child status surface %#v", parentEvents)
	}
	select {
	case job := <-done:
		if job.ID != spawned.SessionID {
			t.Fatalf("unexpected propagated job %#v", job)
		}
	case <-time.After(time.Second):
		t.Fatal("async task did not propagate completion")
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "make a plan", Completion: acp.CompletionInline, PlanRequested: true}); err != nil {
		t.Fatal(err)
	}
	job, err = manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Plan) != 2 || job.Plan[0].Status != "completed" || job.Plan[1].Status != "in_progress" {
		t.Fatalf("unexpected plan %#v", job.Plan)
	}
	if job.Modes.CurrentModeID != "plan" {
		t.Fatalf("current mode = %q, want plan", job.Modes.CurrentModeID)
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "approved", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err = manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.Modes.CurrentModeID != "full-access" {
		t.Fatalf("current mode after approval = %q, want full-access", job.Modes.CurrentModeID)
	}
}

func hasACPMessage(events []sessionevents.Event, content string) bool {
	return countACPMessage(events, content) > 0
}

func countACPMessage(events []sessionevents.Event, content string) int {
	count := 0
	for _, event := range events {
		if event.Type == "acp_message" && event.Content == content {
			count++
		}
	}
	return count
}

func hasACPTool(events []sessionevents.Event, title string) bool {
	for _, event := range events {
		if event.Type != "acp_tool" || event.ACP == nil {
			continue
		}
		for _, call := range event.ACP.ToolCalls {
			if call.Title == title {
				return true
			}
		}
	}
	return false
}

func hasACPStatus(events []sessionevents.Event, id string) bool {
	for _, event := range events {
		if event.Type == "acp" && event.ACP != nil && event.ACP.ID == id {
			return true
		}
	}
	return false
}

func TestFakeACPAgentProcess(t *testing.T) {
	if os.Getenv("JAZ_FAKE_ACP_AGENT") != "1" {
		return
	}
	conn := stdio.New(os.Stdin, os.Stdout)
	currentMode := "auto"
	for {
		msg, err := conn.Receive(context.Background())
		if err != nil {
			os.Exit(0)
		}
		if !msg.IsRequest() {
			continue
		}
		switch msg.Method {
		case "initialize":
			sendResult(conn, msg, map[string]any{
				"protocolVersion": 1,
				"agentInfo":       map[string]any{"name": "fake-agent", "version": "test"},
				"agentCapabilities": map[string]any{
					"loadSession": false,
				},
			})
		case "session/new":
			var req struct {
				Meta map[string]any `json:"_meta"`
			}
			if err := json.Unmarshal(msg.Params, &req); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("invalid session/new params", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if want := os.Getenv("JAZ_FAKE_ACP_SYSTEM_PROMPT"); want != "" && req.Meta["systemPrompt"] != want {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("missing system prompt", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			sendResult(conn, msg, map[string]any{
				"sessionId": "fake-session",
				"modes": map[string]any{
					"currentModeId": "auto",
					"availableModes": []map[string]any{
						{"id": "auto", "name": "Auto"},
						{"id": "full-access", "name": "Full Access"},
						{"id": "plan", "name": "Plan"},
					},
				},
			})
		case "session/set_mode":
			var req struct {
				ModeID string `json:"modeId"`
			}
			if err := json.Unmarshal(msg.Params, &req); err != nil || (req.ModeID != "full-access" && req.ModeID != "plan") {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected supported mode", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			currentMode = req.ModeID
			sendResult(conn, msg, map[string]any{})
		case "session/prompt":
			if currentMode == "plan" {
				notify(conn, "session/update", map[string]any{
					"sessionId": "fake-session",
					"update": map[string]any{
						"sessionUpdate": "plan",
						"entries": []map[string]any{
							{"content": "Inspect request", "priority": "high", "status": "completed"},
							{"content": "Wait for approval", "priority": "medium", "status": "in_progress"},
						},
					},
				})
				sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
				continue
			}
			notify(conn, "session/update", map[string]any{
				"sessionId": "fake-session",
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]any{"type": "text", "text": "hello from fake agent"},
				},
			})
			notify(conn, "session/update", map[string]any{
				"sessionId": "fake-session",
				"update": map[string]any{
					"sessionUpdate": "tool_call",
					"toolCallId":    "tool-1",
					"title":         "whoami",
					"status":        "completed",
				},
			})
			sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
		default:
			resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.MethodNotFound(msg.Method))
			_ = conn.Send(context.Background(), resp)
		}
	}
}

func sendResult(conn jsonrpc.MessageConn, req *jsonrpc.Message, result any) {
	resp, err := jsonrpc.NewResult(*req.ID, result)
	if err == nil {
		_ = conn.Send(context.Background(), resp)
	}
}

func notify(conn jsonrpc.MessageConn, method string, params any) {
	if _, err := json.Marshal(params); err != nil {
		return
	}
	msg, err := jsonrpc.NewNotification(method, params)
	if err == nil {
		_ = conn.Send(context.Background(), msg)
	}
}
