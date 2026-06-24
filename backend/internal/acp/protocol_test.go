package acp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestRequestPermissionPublishesAndWaitsForAnswer(t *testing.T) {
	root := t.TempDir()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "permission", Runtime: storage.RuntimeACP})
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
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, ACPSession: "acp-session", Cwd: root}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

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
				Options: []acpschema.PermissionOption{{
					OptionID: "approve",
					Name:     "Approve tool",
					Kind:     acpschema.PermissionOptionKindAllowOnce,
				}},
				ToolCall: acpschema.ToolCallUpdate{
					ToolCallID: "tool-approval",
					Title:      "Approve tool",
					Locations:  []acpschema.ToolCallLocation{{Path: filepath.Join(root, "index.html")}},
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
		if event.Permission.ToolCallID != "tool-approval" || len(event.Permission.Locations) != 1 {
			t.Fatalf("permission = %#v", event.Permission)
		}
		loaded, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded.LastAttentionAt.After(oldAttention) {
			t.Fatalf("permission request did not advance attention: %s -> %s", oldAttention, loaded.LastAttentionAt)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{Session: session.ID, RequestID: requestID, OptionID: "approve"}); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-result:
		var got map[string]map[string]string
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got["outcome"]["outcome"] != "selected" || got["outcome"]["optionId"] != "approve" {
			t.Fatalf("unexpected permission response: %s", raw)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
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

func TestAppendACPTextPreservesProviderChunks(t *testing.T) {
	if got := appendACPText("Done.", "Next"); got != "Done.Next" {
		t.Fatalf("appendACPText inserted formatting: %q", got)
	}
	if got := appendACPText("Line one", "\n\nLine two"); got != "Line one\n\nLine two" {
		t.Fatalf("appendACPText changed provider whitespace: %q", got)
	}
}

func TestInteractiveRequestUserInputPublishesStructuredQuestions(t *testing.T) {
	runInteractiveRequestUserInputTest(t, codexRequestUserInputMetaKey, "__user_input_submit__", userInputResponseOptionPrefix)
}

func runInteractiveRequestUserInputTest(t *testing.T, metaKey, submitOptionID, responseOptionPrefix string) {
	t.Helper()

	root := t.TempDir()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID["session"] = &jobState{Job: Job{ID: "session", ACPSession: "acp-session", Cwd: root}}
	manager.jobsByACP["acp-session"] = manager.jobsByID["session"]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, "session")
	result := make(chan json.RawMessage, 1)
	errs := make(chan *jsonrpc.Error, 1)

	go func() {
		raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
			Method: acpschema.ClientMethodSessionRequestPermission,
			Params: mustJSON(t, acpschema.RequestPermissionRequest{
				SessionID: "acp-session",
				Options: []acpschema.PermissionOption{{
					OptionID: acpschema.PermissionOptionID(submitOptionID),
					Name:     "Submit answers",
					Kind:     acpschema.PermissionOptionKindAllowOnce,
				}},
				ToolCall: acpschema.ToolCallUpdate{
					ToolCallID: "request-user-input-call-1",
					Title:      "Clarifying questions",
					Meta: map[string]any{
						metaKey: map[string]any{
							"call_id": "call-1",
							"turn_id": "turn-1",
							"questions": []map[string]any{
								{
									"id":       "audience",
									"question": "Who is the audience?",
									"options": []map[string]any{
										{"label": "Kids", "description": "Use simpler copy."},
									},
								},
							},
						},
					},
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
		if len(event.Permission.Questions) != 1 || event.Permission.Questions[0].ID != "audience" {
			t.Fatalf("questions = %#v", event.Permission.Questions)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	state, err := store.LoadACPState("session")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Permissions) != 0 {
		t.Fatalf("persisted permissions = %#v", state.Permissions)
	}
	if len(manager.jobsByID["session"].Permissions) != 1 ||
		len(manager.jobsByID["session"].Permissions[0].Questions) != 1 ||
		manager.jobsByID["session"].Permissions[0].Questions[0].ID != "audience" {
		t.Fatalf("live permissions = %#v", manager.jobsByID["session"].Permissions)
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{
		Session:   "session",
		RequestID: requestID,
		Answers: map[string]InteractiveAnswerValue{
			"audience": {Answers: []string{"Kids"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-result:
		var got map[string]map[string]string
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		optionID := got["outcome"]["optionId"]
		if got["outcome"]["outcome"] != "selected" || !strings.HasPrefix(optionID, responseOptionPrefix) {
			t.Fatalf("unexpected user input response: %s", raw)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
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

	state, err := store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Plan) != 2 || state.Plan[1].Status != "in_progress" {
		t.Fatalf("persisted progress = %#v", state.Plan)
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

func TestToolCallUpdateCapturesLivenessState(t *testing.T) {
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
		Job:      Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"},
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
				"rawInput":      map[string]any{"cmd": "go test ./..."},
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

	select {
	case event := <-sub:
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
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	state, err := store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastEventAt.IsZero() || state.LastToolAt.IsZero() {
		t.Fatalf("state timestamps missing: %#v", state)
	}
	if len(state.ToolCalls) != 1 || state.ToolCalls[0].Runtime.TerminalID != "exec-1" {
		t.Fatalf("persisted tool call = %#v", state.ToolCalls)
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

	state, err := store.LoadACPState(session.ID)
	if err != nil && !strings.Contains(err.Error(), "acp state not found") {
		t.Fatal(err)
	}
	if err == nil && len(state.Plan) != 0 {
		t.Fatalf("persisted progress = %#v", state.Plan)
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

	state, err := store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Plan) != 0 {
		t.Fatalf("persisted progress = %#v", state.Plan)
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
					Kind:       ptr(acpschema.ToolKindSwitchMode),
					ToolCallID: "toolu-plan-exit",
					Title:      "Ready to code?",
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
		if event.Permission.ToolCallID != "toolu-plan-exit" || len(event.Permission.Options) != 4 {
			t.Fatalf("permission = %#v", event.Permission)
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
