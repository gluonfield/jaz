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
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestRequestPermissionApprovesWorkspaceLocalTool(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(nil, Config{})
	manager.jobsByACP["acp-session"] = &Job{ID: "session", ACPSession: "acp-session", Cwd: root}

	raw, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: acpschema.ClientMethodSessionRequestPermission,
		Params: mustJSON(t, acpschema.RequestPermissionRequest{
			SessionID: "acp-session",
			Options:   []acpschema.PermissionOption{{OptionID: "allow_once", Name: "Allow once"}},
			ToolCall: acpschema.ToolCallUpdate{
				Locations: []acpschema.ToolCallLocation{{Path: filepath.Join(root, "index.html")}},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	var got map[string]map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["outcome"]["outcome"] != "selected" || got["outcome"]["optionId"] != "allow_once" {
		t.Fatalf("unexpected permission response: %s", raw)
	}
}

func TestRequestPermissionCancelsWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(nil, Config{})
	manager.jobsByACP["acp-session"] = &Job{ID: "session", ACPSession: "acp-session", Cwd: root}

	raw, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: acpschema.ClientMethodSessionRequestPermission,
		Params: mustJSON(t, acpschema.RequestPermissionRequest{
			SessionID: "acp-session",
			Options:   []acpschema.PermissionOption{{OptionID: "allow_once", Name: "Allow once"}},
			ToolCall: acpschema.ToolCallUpdate{
				Locations: []acpschema.ToolCallLocation{{Path: filepath.Join(root, "..", "outside")}},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	var got map[string]map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["outcome"]["outcome"] != "cancelled" {
		t.Fatalf("unexpected permission response: %s", raw)
	}
}

func TestInteractiveRequestPermissionWaitsForAnswer(t *testing.T) {
	root := t.TempDir()
	events := sessionevents.New()
	manager := NewManager(nil, Config{})
	manager.Events = events
	manager.jobsByID["session"] = &Job{ID: "session", ACPSession: "acp-session", Cwd: root, interactive: true}
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
					OptionID: "approve",
					Name:     "Approve plan",
					Kind:     acpschema.PermissionOptionKindAllowOnce,
				}},
				ToolCall: acpschema.ToolCallUpdate{ToolCallID: "plan-approval", Title: "Approve plan"},
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
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{Session: "session", RequestID: requestID, OptionID: "approve"}); err != nil {
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

func TestInteractiveRequestUserInputPublishesStructuredQuestions(t *testing.T) {
	root := t.TempDir()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{})
	manager.Events = events
	manager.jobsByID["session"] = &Job{ID: "session", ACPSession: "acp-session", Cwd: root, interactive: true}
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
					OptionID: "__jaz_user_input_submit__",
					Name:     "Submit answers",
					Kind:     acpschema.PermissionOptionKindAllowOnce,
				}},
				ToolCall: acpschema.ToolCallUpdate{
					ToolCallID: "request-user-input-call-1",
					Title:      "Clarifying questions",
					Meta: map[string]any{
						codexRequestUserInputMetaKey: map[string]any{
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
	if len(state.Permissions) != 1 || len(state.Permissions[0].Questions) != 1 || state.Permissions[0].Questions[0].ID != "audience" {
		t.Fatalf("persisted permissions = %#v", state.Permissions)
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
		if got["outcome"]["outcome"] != "selected" || !strings.HasPrefix(optionID, userInputResponseOptionPrefix) {
			t.Fatalf("unexpected user input response: %s", raw)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestPlanSessionUpdatePublishesAndPersistsPlan(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{})
	manager.Events = events
	manager.jobsByID["session"] = &Job{ID: "session", ACPSession: "acp-session", Slug: "claude-plan"}
	manager.jobsByACP["acp-session"] = manager.jobsByID["session"]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, "session")

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
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
	})
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
			t.Fatalf("plan event = %#v", event.ACP.Plan)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	state, err := store.LoadACPState("session")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Plan) != 2 || state.Plan[1].Status != "in_progress" {
		t.Fatalf("persisted plan = %#v", state.Plan)
	}
}

func TestClaudeStylePlanExitPermissionPublishesGenericPermission(t *testing.T) {
	root := t.TempDir()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{})
	manager.Events = events
	manager.jobsByID["session"] = &Job{ID: "session", ACPSession: "acp-session", Cwd: root, interactive: true}
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
				Options: []acpschema.PermissionOption{
					{OptionID: "default", Name: "Yes, and manually approve edits", Kind: acpschema.PermissionOptionKindAllowOnce},
					{OptionID: "plan", Name: "No, keep planning", Kind: acpschema.PermissionOptionKindRejectOnce},
				},
				ToolCall: acpschema.ToolCallUpdate{
					ToolCallID: "exit-plan-mode",
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
		if event.Permission.Title != "Ready to code?" {
			t.Fatalf("title = %q", event.Permission.Title)
		}
		if len(event.Permission.Questions) != 0 {
			t.Fatalf("Claude-style permission should not synthesize questions: %#v", event.Permission.Questions)
		}
		if len(event.Permission.Options) != 2 || event.Permission.Options[1].ID != "plan" {
			t.Fatalf("options = %#v", event.Permission.Options)
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
	if len(state.Permissions) != 1 || state.Permissions[0].Title != "Ready to code?" {
		t.Fatalf("persisted permissions = %#v", state.Permissions)
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{
		Session:   "session",
		RequestID: requestID,
		OptionID:  "plan",
	}); err != nil {
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
