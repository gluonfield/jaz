package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestACPTransportDecodesChunkMessageID(t *testing.T) {
	update, err := acpschema.DecodeSessionUpdate(json.RawMessage(`{
		"sessionUpdate": "agent_message_chunk",
		"messageId": "provider-message-1",
		"content": {"type": "text", "text": "hello"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	message, ok := update.(acpschema.AgentMessageChunkUpdate)
	if !ok {
		t.Fatalf("decoded %T, want AgentMessageChunkUpdate", update)
	}
	if got := derefString(message.MessageID); got != "provider-message-1" {
		t.Fatalf("messageId = %q, want provider-message-1", got)
	}
}

func TestAgentMessageChunkMessageIDPersistsTextRunID(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "message-id", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job

	_, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"messageId":     "provider-message-1",
				"content":       map[string]any{"type": "text", "text": "hello"},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	manager.withACPTranscriptBarrier(job.eventView(), nil)

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Type != sessionevents.TypeACPMessage {
		t.Fatalf("stored events = %#v", stored)
	}
	if stored[0].Content != "hello" || stored[0].ACP.TextRunID != "message:provider-message-1" {
		t.Fatalf("stored message event = %#v", stored[0])
	}
}

func TestCodexHiddenWarningsDoNotPolluteAssistantAnswer(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "warning", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	job := &jobState{Job: Job{ID: session.ID, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job

	for _, message := range []struct{ id, text string }{
		{"", "Warning: Falling back from WebSockets to HTTPS transport. disconnected\n\n"},
		{"codex:warning:turn:1", "Falling back from WebSockets to HTTPS transport. disconnected"},
		{"https-answer", "done"},
	} {
		update := map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]any{"type": "text", "text": message.text},
		}
		if message.id != "" {
			update["messageId"] = message.id
		}
		_, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
			Method: acpschema.ClientMethodSessionUpdate,
			Params: mustJSON(t, map[string]any{"sessionId": "acp-session", "update": update}),
		})
		if rpcErr != nil {
			t.Fatal(rpcErr)
		}
	}
	manager.withACPTranscriptBarrier(job.eventView(), nil)

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Type != sessionevents.TypeACPMessage || stored[0].Content != "done" || job.Assistant != "done" {
		t.Fatalf("stored = %#v, assistant = %q", stored, job.Assistant)
	}
}

// Codex sends warnings as plain message chunks without a message id, so
// recognition rests on the text and the agent alone.
func TestCodexHiddenWarningsNeedNoMessageIDAndStayCodexOnly(t *testing.T) {
	update, err := acpschema.DecodeSessionUpdate(json.RawMessage(`{
		"sessionUpdate": "agent_message_chunk",
		"content": {"type": "text", "text": "Warning: Falling back from WebSockets to HTTPS transport. disconnected\n\n"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !codexHiddenWarning(AgentCodex, update) {
		t.Fatal("codex transport fallback was not recognized")
	}
	if codexHiddenWarning(AgentClaude, update) {
		t.Fatal("non-codex output was suppressed")
	}
	answer, err := acpschema.DecodeSessionUpdate(json.RawMessage(`{
		"sessionUpdate": "agent_message_chunk",
		"content": {"type": "text", "text": "answer"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if codexHiddenWarning(AgentCodex, answer) {
		t.Fatal("ordinary codex output was suppressed")
	}
}

// The fallback-metadata warning is the only signal that a session is running on
// guessed model metadata, so it must reach the transcript.
func TestCodexModelMetadataWarningStaysVisible(t *testing.T) {
	update, err := acpschema.DecodeSessionUpdate(json.RawMessage(`{
		"sessionUpdate": "agent_message_chunk",
		"content": {"type": "text", "text": "Warning: Model metadata for ` + "`gpt-5.6-sol`" + ` not found. Defaulting to fallback metadata; this can degrade performance and cause issues.\n\n"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if codexHiddenWarning(AgentCodex, update) {
		t.Fatal("fallback metadata warning was suppressed")
	}
}

func TestRequestPermissionUnknownSessionErrors(t *testing.T) {
	manager := NewManager(nil, Config{}, nil)

	_, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: acpschema.ClientMethodSessionRequestPermission,
		Params: mustJSON(t, acpschema.RequestPermissionRequest{
			SessionID: "unknown-acp-session",
			ToolCall:  acpschema.ToolCallUpdate{ToolCallID: "tool", Title: "Tool"},
		}),
	})
	if rpcErr == nil {
		t.Fatal("expected unknown session error")
	}
}

func TestJobTextBuffersPreserveProviderChunks(t *testing.T) {
	job := &jobState{}
	job.appendAssistantLocked("Done.")
	job.appendAssistantLocked("Next")
	job.appendThoughtLocked("Line one")
	job.appendThoughtLocked("\n\nLine two")
	if job.Assistant != "Done.Next" {
		t.Fatalf("assistant text = %q", job.Assistant)
	}
	if job.Thought != "Line one\n\nLine two" {
		t.Fatalf("thought text = %q", job.Thought)
	}
}

func TestPlanSessionUpdatePublishesAndPersistsProgress(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-plan", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	oldAttention := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	storage.MarkSessionAttention(&session, oldAttention)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, ACPSession: "acp-session", Slug: session.Slug, State: StateRunning}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	request := jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "plan",
				"entries": []map[string]any{
					{"content": "Inspect existing page", "status": "completed", "priority": "medium"},
					{"content": "Draft implementation plan", "status": "in_progress", "priority": "medium"},
				},
			},
		}),
	}
	raw, rpcErr := manager.handleJSONRPC(ctx, request)
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	select {
	case event := <-sub:
		if event.Type != "acp" || event.ACP == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		if len(event.ACP.Plan) != 2 || event.ACP.Plan[0].Content != "Inspect existing page" {
			t.Fatalf("progress event = %#v", event.ACP.Plan)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	storedEvents, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedEvents) != 1 || storedEvents[0].ACP == nil || len(storedEvents[0].ACP.Plan) != 2 {
		t.Fatalf("persisted progress events = %#v", storedEvents)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LastAttentionAt.After(oldAttention) {
		t.Fatalf("progress update did not advance attention: %s -> %s", oldAttention, loaded.LastAttentionAt)
	}
	attentionAfterProgress := loaded.LastAttentionAt
	if _, rpcErr := manager.handleJSONRPC(ctx, request); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LastAttentionAt.Equal(attentionAfterProgress) {
		t.Fatalf("unchanged progress advanced attention: %s -> %s", attentionAfterProgress, loaded.LastAttentionAt)
	}
}

func TestSideChatSessionUpdatePublishesAndPersistsSideChatEvent(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-side", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"messageId":     "side-chat-message",
				"_meta": map[string]any{
					"codex": map[string]any{
						"sideChat": map[string]any{
							"id":              "side-1",
							"command":         "side",
							"parentSessionId": session.ID,
							"threadId":        "thread-1",
						},
					},
				},
				"content": map[string]any{"type": "text", "text": "side answer"},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	select {
	case event := <-sub:
		if event.Type != sessionevents.TypeSideChatMessage || event.SideChat == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		if event.Content != "" ||
			event.SideChat.Content != "side answer" ||
			event.SideChat.ID != "side-1" ||
			event.SideChat.ThreadID != "thread-1" ||
			event.SideChat.Role != "assistant" {
			t.Fatalf("side chat event = %#v", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got := manager.jobsByID[session.ID].Assistant; got != "" {
		t.Fatalf("main assistant was mutated: %q", got)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Type != sessionevents.TypeSideChatMessage || stored[0].SideChat == nil {
		t.Fatalf("stored events = %#v", stored)
	}
	if stored[0].SideChat.Content != "side answer" || stored[0].SideChat.ID != "side-1" {
		t.Fatalf("stored side chat event = %#v", stored[0])
	}
}

func TestToolCallUpdateCapturesLivenessInEvent(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-tools", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{
		Job:      Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session", State: StateRunning},
		toolByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    "exec-1",
				"title":         "go test ./...",
				"status":        "in_progress",
				"_meta": map[string]any{
					"terminal_info": map[string]any{"terminal_id": "exec-1", "cwd": "/repo"},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	event := receiveEvent(t, sub)
	if event.Type != "acp_tool" || event.ACP == nil || len(event.ACP.ToolCalls) != 1 {
		t.Fatalf("unexpected event %#v", event)
	}
	call := event.ACP.ToolCalls[0]
	if call.Runtime.TerminalID != "exec-1" || call.Runtime.TerminalCwd != "/repo" {
		t.Fatalf("runtime = %#v", call.Runtime)
	}
	if call.StartedAt.IsZero() || call.UpdatedAt.IsZero() || event.ACP.LastToolAt.IsZero() {
		t.Fatalf("timestamps missing: call=%#v acp=%#v", call, event.ACP)
	}

	storedEvents, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedEvents) != 1 || storedEvents[0].ACP == nil || len(storedEvents[0].ACP.ToolCalls) != 1 || storedEvents[0].ACP.ToolCalls[0].Runtime.TerminalID != "exec-1" {
		t.Fatalf("persisted tool events = %#v", storedEvents)
	}
}

func TestToolCallUpdateDropsTransportOnlySnapshots(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "tool-snapshots", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{
		Job:      Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentGrok, ACPSession: "acp-session"},
		toolByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job

	update := func(status string) {
		t.Helper()
		_, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
			Method: acpschema.ClientMethodSessionUpdate,
			Params: mustJSON(t, map[string]any{
				"sessionId": "acp-session",
				"update": map[string]any{
					"sessionUpdate": "tool_call_update",
					"toolCallId":    "harbor-1",
					"title":         "Run Harbor",
					"status":        status,
					"rawOutput":     map[string]any{"state": "running"},
					"_meta":         map[string]any{"terminal_output": true},
				},
			}),
		})
		if rpcErr != nil {
			t.Fatal(rpcErr)
		}
	}

	update("in_progress")
	update("in_progress")
	update("completed")
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored events = %d, want initial and semantic status update: %#v", len(stored), stored)
	}
	if got := stored[1].ACP.ToolCalls[0].Status; got != "completed" {
		t.Fatalf("final status = %q, want completed", got)
	}
}

func TestSparseToolCallUpdateWaitsForToolData(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-progress", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{
		Job:                   Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentClaude, ACPSession: "acp-session"},
		toolByID:              map[string]sessionevents.ACPToolCall{},
		pendingToolUpdateByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    "exec-1",
				"status":        "in_progress",
				"_meta": map[string]any{
					"claudeCode": map[string]any{
						"toolName":     "Bash",
						"toolResponse": map[string]any{"elapsedTimeSeconds": 1},
					},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}
	assertNoEvent(t, sub)
	if got := manager.jobsByID[session.ID].ToolCalls; len(got) != 0 {
		t.Fatalf("orphan update created tool calls = %#v", got)
	}

	_, rpcErr = manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    "exec-1",
				"kind":          "execute",
				"status":        "in_progress",
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	assertNoEvent(t, sub)
	if got := manager.jobsByID[session.ID].ToolCalls; len(got) != 0 {
		t.Fatalf("kind-only update created tool calls = %#v", got)
	}

	_, rpcErr = manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    "exec-1",
				"title":         "ls",
				"status":        "completed",
				"rawInput":      map[string]any{"command": "ls"},
				"rawOutput":     map[string]any{"stdout": "ok"},
				"locations":     []map[string]any{{"path": "script.sh", "line": 7}},
				"_meta": map[string]any{
					"claudeCode": map[string]any{"toolName": "Bash"},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	event := receiveEvent(t, sub)
	if event.Type != "acp_tool" || event.ACP == nil || len(event.ACP.ToolCalls) != 1 {
		t.Fatalf("unexpected event %#v", event)
	}
	call := event.ACP.ToolCalls[0]
	if call.Title != "ls" || call.ToolName != "Bash" || call.RawInput["command"] != "ls" {
		t.Fatalf("tool call = %#v", call)
	}
	if call.Status != "completed" {
		t.Fatalf("status = %q, want completed", call.Status)
	}
	if len(call.Locations) != 1 || call.Locations[0].Path != "script.sh" || call.Locations[0].Line != 7 {
		t.Fatalf("locations = %#v", call.Locations)
	}
	if !strings.Contains(string(call.RawOutput), "ok") {
		t.Fatalf("raw output = %s", call.RawOutput)
	}
	if call.Runtime.ElapsedTimeSeconds != 1 {
		t.Fatalf("elapsed seconds = %v, want 1", call.Runtime.ElapsedTimeSeconds)
	}
}

func TestRawOutputToolCallUpdateMaterializesToolData(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "tool-result", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{
		Job:                   Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentClaude, ACPSession: "acp-session"},
		toolByID:              map[string]sessionevents.ACPToolCall{},
		pendingToolUpdateByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    "result-1",
				"status":        "completed",
				"rawOutput":     map[string]any{"stdout": "done"},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	event := receiveEvent(t, sub)
	if event.Type != "acp_tool" || event.ACP == nil || len(event.ACP.ToolCalls) != 1 {
		t.Fatalf("unexpected event %#v", event)
	}
	call := event.ACP.ToolCalls[0]
	if call.ID != "result-1" || call.Status != "completed" || !strings.Contains(string(call.RawOutput), "done") {
		t.Fatalf("tool call = %#v", call)
	}
}

func TestPlanSessionUpdateIgnoresMarkdownDocumentEntry(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-progress", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	oldAttention := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	storage.MarkSessionAttention(&session, oldAttention)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, ACPSession: "acp-session", Slug: session.Slug}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	request := jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "plan",
				"entries": []map[string]any{{
					"content":  "# Keep `npx` for Built-In ACP Adapters\n\n## Summary\n- Do not add managed downloads yet.",
					"status":   "in_progress",
					"priority": "medium",
				}},
			},
		}),
	}
	raw, rpcErr := manager.handleJSONRPC(ctx, request)
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	select {
	case event := <-sub:
		t.Fatalf("unexpected event %#v", event)
	case <-time.After(100 * time.Millisecond):
	}

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LastAttentionAt.Equal(oldAttention) {
		t.Fatalf("invalid progress advanced attention: %s -> %s", oldAttention, loaded.LastAttentionAt)
	}
}

func TestPlanSessionUpdateInvalidReplacementClearsProgress(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-progress", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, ACPSession: "acp-session", Slug: session.Slug}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	valid := jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "plan",
				"entries": []map[string]any{{
					"content":  "Inspect existing page",
					"status":   "in_progress",
					"priority": "medium",
				}},
			},
		}),
	}
	if _, rpcErr := manager.handleJSONRPC(ctx, valid); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	select {
	case event := <-sub:
		if len(event.ACP.Plan) != 1 {
			t.Fatalf("valid progress event = %#v", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	invalid := jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "plan",
				"entries": []map[string]any{{
					"content":  "# Full markdown plan\n\n- one\n- two",
					"status":   "in_progress",
					"priority": "medium",
				}},
			},
		}),
	}
	if _, rpcErr := manager.handleJSONRPC(ctx, invalid); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	select {
	case event := <-sub:
		if event.ACP == nil || event.ACP.Plan == nil || len(event.ACP.Plan) != 0 {
			t.Fatalf("clear progress event = %#v", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

}

func TestSessionInfoUpdatePublishesAndPersistsTitle(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-task",
		Title:   "old title",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{
		Job: Job{
			ID:         session.ID,
			Slug:       session.Slug,
			Title:      session.Title,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			State:      StateRunning,
		},
	}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "session_info_update",
				"title":         "Derived ACP title",
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	select {
	case event := <-sub:
		if event.Type != "acp" || event.ACP == nil || event.ACP.Title != "Derived ACP title" {
			t.Fatalf("unexpected event %#v", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "Derived ACP title" {
		t.Fatalf("stored title = %q", loaded.Title)
	}
}

func TestSessionInfoUpdateDoesNotOverwriteManualTitle(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-task",
		Title:   "old title",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{
		Job: Job{
			ID:         session.ID,
			Slug:       session.Slug,
			Title:      session.Title,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			State:      StateRunning,
		},
	}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	if err := store.UpdateSessionTitle(session.ID, "Manual title"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "session_info_update",
				"title":         "Derived ACP title",
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	select {
	case event := <-sub:
		t.Fatalf("unexpected title event after manual rename: %#v", event)
	default:
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "Manual title" {
		t.Fatalf("stored title = %q, want manual title", loaded.Title)
	}
	if got := manager.jobsByID[session.ID].Snapshot().Title; got != "Manual title" {
		t.Fatalf("job title = %q, want manual title", got)
	}
}

func TestSessionInfoUpdateManualTitleSurvivesRepeatedRuntimeTitles(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-task",
		Title:   "old title",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.jobsByID[session.ID] = &jobState{
		Job: Job{
			ID:         session.ID,
			Slug:       session.Slug,
			Title:      session.Title,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			State:      StateRunning,
		},
	}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	sendTitle := func(title string) {
		t.Helper()
		raw, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
			Method: acpschema.ClientMethodSessionUpdate,
			Params: mustJSON(t, map[string]any{
				"sessionId": "acp-session",
				"update": map[string]any{
					"sessionUpdate": "session_info_update",
					"title":         title,
				},
			}),
		})
		if rpcErr != nil {
			t.Fatal(rpcErr)
		}
		if string(raw) != "{}" {
			t.Fatalf("response = %s", raw)
		}
	}

	sendTitle("Derived ACP title")
	if err := store.UpdateSessionTitle(session.ID, "Manual title"); err != nil {
		t.Fatal(err)
	}
	sendTitle("Derived ACP title")
	sendTitle("Another ACP title")

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "Manual title" || !loaded.ManualTitle {
		t.Fatalf("stored title/manual = %q/%v, want manual title", loaded.Title, loaded.ManualTitle)
	}
	if got := manager.jobsByID[session.ID].Snapshot().Title; got != "Manual title" {
		t.Fatalf("job title = %q, want manual title", got)
	}
}

func TestClaudeStylePlanExitPermissionPublishesRequest(t *testing.T) {
	root := t.TempDir()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-plan-exit", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	oldAttention := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	storage.MarkSessionAttention(&session, oldAttention)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{
		Job: Job{
			ID:         session.ID,
			ACPAgent:   AgentClaude,
			ACPSession: "acp-session",
			Cwd:        root,
			Modes: ModeState{
				CurrentModeID: "plan",
				PlanModeID:    "plan",
				AvailableModes: []ModeSnapshot{
					{ID: "bypassPermissions", Name: "Bypass Permissions"},
					{ID: "auto", Name: "Auto"},
					{ID: "plan", Name: "Plan"},
				},
			},
		},
	}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]
	// Claude 0.84 names the kind only in the tool_call; its permission request
	// leaves unchanged fields out.
	if _, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: json.RawMessage(`{"sessionId":"acp-session","update":{"sessionUpdate":"tool_call","toolCallId":"toolu-plan-exit","title":"Approve Plan","kind":"switch_mode","status":"pending","content":[]}}`),
	}); rpcErr != nil {
		t.Fatal(rpcErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	result := make(chan json.RawMessage, 1)
	errs := make(chan *jsonrpc.Error, 1)

	go func() {
		raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
			Method: acpschema.ClientMethodSessionRequestPermission,
			Params: mustJSON(t, acpschema.RequestPermissionRequest{
				SessionID: "acp-session",
				Options: []acpschema.PermissionOption{
					{OptionID: "default", Name: "Yes, and manually approve edits", Kind: acpschema.PermissionOptionKindAllowOnce},
					{OptionID: "auto", Name: `Yes, and use "auto" mode`, Kind: acpschema.PermissionOptionKindAllowAlways},
					{OptionID: "bypassPermissions", Name: "Yes, and bypass permissions", Kind: acpschema.PermissionOptionKindAllowAlways},
					{OptionID: "plan", Name: "No, keep planning", Kind: acpschema.PermissionOptionKindRejectOnce},
				},
				ToolCall: acpschema.ToolCallUpdate{
					ToolCallID: "toolu-plan-exit",
					Title:      "Approve Plan",
					RawInput:   mustJSON(t, map[string]string{"plan": "1. Inspect the provider path.\n2. Add the flag."}),
				},
			}),
		})
		if rpcErr != nil {
			errs <- rpcErr
			return
		}
		result <- raw
	}()

	var requestID string
	select {
	case event := <-sub:
		if event.Type != "permission_request" || event.Permission == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		requestID = event.Permission.ID
		loaded, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded.LastAttentionAt.After(oldAttention) {
			t.Fatalf("plan permission did not advance attention: %s -> %s", oldAttention, loaded.LastAttentionAt)
		}
		if event.Permission.ToolCallID != "toolu-plan-exit" || len(event.Permission.Options) != 4 {
			t.Fatalf("permission = %#v", event.Permission)
		}
		if event.Permission.Content != "1. Inspect the provider path.\n2. Add the flag." {
			t.Fatalf("permission plan content = %q", event.Permission.Content)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{Session: session.ID, RequestID: requestID, OptionID: "plan"}); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-result:
		var got map[string]map[string]string
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got["outcome"]["outcome"] != "selected" || got["outcome"]["optionId"] != "plan" {
			t.Fatalf("unexpected permission response: %s", raw)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func ptr[T any](value T) *T {
	return &value
}

// Codex streams command output per chunk; output-only updates are coalesced
// into one deferred publish, and a status change still publishes at once.
func TestTerminalOutputUpdatesAreCoalesced(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "coalesced-output", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, ACPAgent: AgentCodex, ACPSession: "acp-session"}, toolByID: map[string]sessionevents.ACPToolCall{}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := manager.Events.Subscribe(ctx, session.ID)
	send := func(update string) {
		if _, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
			Method: acpschema.ClientMethodSessionUpdate,
			Params: json.RawMessage(`{"sessionId":"acp-session","update":` + update + `}`),
		}); rpcErr != nil {
			t.Fatal(rpcErr)
		}
	}
	next := func() sessionevents.ACPToolCall {
		for {
			select {
			case event := <-sub:
				if event.Type == "acp_tool" {
					return event.ACP.ToolCalls[0]
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	send(`{"sessionUpdate":"tool_call","toolCallId":"cmd-1","title":"npm test","kind":"execute","status":"in_progress","content":[{"type":"terminal","terminalId":"cmd-1"}],"rawInput":{"command":"npm test"},"_meta":{"terminal_info":{"terminal_id":"cmd-1"}}}`)
	if call := next(); call.Runtime.TerminalOutput != "" {
		t.Fatalf("started call = %+v", call)
	}
	for _, line := range []string{"one", "two", "three"} {
		send(`{"sessionUpdate":"tool_call_update","toolCallId":"cmd-1","_meta":{"terminal_output_delta":{"terminal_id":"cmd-1","data":"` + line + `\n"}}}`)
	}
	if call := next(); call.Runtime.TerminalOutput != "one\ntwo\nthree\n" || call.Status != "in_progress" {
		t.Fatalf("coalesced call = %+v", call)
	}
	send(`{"sessionUpdate":"tool_call_update","toolCallId":"cmd-1","status":"completed","_meta":{"terminal_exit":{"terminal_id":"cmd-1","exit_code":0,"signal":null}}}`)
	if call := next(); call.Status != "completed" || call.Runtime.TerminalOutput != "one\ntwo\nthree\n" {
		t.Fatalf("completed call = %+v", call)
	}
	select {
	case event := <-sub:
		if event.Type == "acp_tool" {
			t.Fatalf("extra tool publish %+v", event.ACP.ToolCalls[0])
		}
	case <-time.After(3 * acpTranscriptFlushInterval):
	}
}
