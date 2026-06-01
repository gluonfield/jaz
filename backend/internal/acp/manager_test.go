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
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1"},
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
	if len(messages) != 2 || provider.MessageContent(messages[1]) != "hello from fake agent" {
		t.Fatalf("unexpected messages %#v", messages)
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

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "again", Completion: acp.CompletionAsync}); err != nil {
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
	if len(messages) != 4 || provider.MessageContent(messages[3]) != "hello from fake agent" {
		t.Fatalf("unexpected follow-up messages %#v", messages)
	}
	select {
	case job := <-done:
		if job.ID != spawned.SessionID {
			t.Fatalf("unexpected propagated job %#v", job)
		}
	case <-time.After(time.Second):
		t.Fatal("async task did not propagate completion")
	}
}

func TestFakeACPAgentProcess(t *testing.T) {
	if os.Getenv("JAZ_FAKE_ACP_AGENT") != "1" {
		return
	}
	conn := stdio.New(os.Stdin, os.Stdout)
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
			sendResult(conn, msg, map[string]any{"sessionId": "fake-session"})
		case "session/prompt":
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
