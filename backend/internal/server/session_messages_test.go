package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionMessagesErrorSessionOverridesRunningACPSnapshot(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-restarted",
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
	if err := store.SaveACPState(session.ID, storage.ACPState{
		ID:         session.ID,
		Slug:       session.Slug,
		ACPAgent:   "codex",
		ACPSession: "acp-session",
		State:      acp.StateRunning,
	}); err != nil {
		t.Fatal(err)
	}
	session, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusError
	session.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: &fakeACPManager{job: acp.Job{
		ID:         session.ID,
		Slug:       session.Slug,
		ACPAgent:   "codex",
		ACPSession: "acp-session",
		State:      acp.StateRunning,
	}}}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Session  storage.Session `json:"session"`
		ACPState string          `json:"acp_state"`
		ACPError string          `json:"acp_error"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Session.Status != storage.StatusError || got.ACPState != acp.StateFailed {
		t.Fatalf("status = %q, acp_state = %q", got.Session.Status, got.ACPState)
	}
	if got.ACPError != session.Error {
		t.Fatalf("acp_error = %q, want %q", got.ACPError, session.Error)
	}
}

func TestCanonicalSessionResponsePublishesPublicGoal(t *testing.T) {
	budget := int64(100)
	data, err := json.Marshal(canonicalSessionResponse(storage.Session{
		ID:      "thread-1",
		Runtime: storage.RuntimeACP,
		Goal: &goal.State{
			Identity: goal.Identity{
				ID:             "goal-1",
				ThreadID:       "thread-1",
				Provider:       "codex",
				ProviderGoalID: "provider-goal-1",
				Objective:      "ship it",
				Status:         goal.StatusActive,
			},
			Budget: goal.Budget{
				TokenBudget: &budget,
				TokensUsed:  25,
			},
			Progress: goal.Progress{
				ProgressMessage: "running tests",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, forbidden := range []string{"provider_goal_id", "progress_message", "budget_source"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("session response leaked goal field %q: %s", forbidden, body)
		}
	}
	for _, required := range []string{`"objective":"ship it"`, `"token_budget":100`, `"tokens_used":25`, `"remaining_tokens":75`} {
		if !strings.Contains(body, required) {
			t.Fatalf("session response missing %s: %s", required, body)
		}
	}
}

func TestSessionEventResponsePublishesPublicGoal(t *testing.T) {
	data, err := json.Marshal(sessionEventResponseFrom(sessionevents.Event{
		SessionID: "thread-1",
		Type:      sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{
			Identity: goal.Identity{
				Objective:      "ship it",
				Status:         goal.StatusActive,
				ProviderGoalID: "provider-goal-1",
			},
			Progress: goal.Progress{
				ProgressMessage: "running tests",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, forbidden := range []string{"provider_goal_id", "progress_message"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("event response leaked goal field %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"objective":"ship it"`) {
		t.Fatalf("event response missing public goal: %s", body)
	}
}

func TestSessionMessagesMobileProjectionStripsHeavyToolPayload(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-mobile",
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
	heavyCall := sessionevents.ACPToolCall{
		ID:       "tool-1",
		Title:    "rg release",
		Status:   "completed",
		Kind:     "terminal",
		ToolName: "shell",
		Content: []sessionevents.ACPToolContent{{
			Type: "text",
			Text: "very large tool result that mobile does not render",
		}},
		RawInput: map[string]any{
			"cmd": "expensive command input that mobile does not render",
		},
		Runtime: sessionevents.ACPToolRuntime{ElapsedTimeSeconds: 12.5},
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: "acp_tool",
		ACP: &sessionevents.ACPEvent{
			ID:        session.ID,
			Slug:      session.Slug,
			Agent:     "codex",
			SessionID: "acp-session",
			State:     acp.StateIdle,
			ToolCalls: []sessionevents.ACPToolCall{heavyCall},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertActivity(session.ID, storage.ActivityEntry{
		ID:     "tool-1",
		Kind:   "tool",
		Text:   "activity entry not rendered by mobile",
		Status: "completed",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveACPState(session.ID, storage.ACPState{
		ID:         session.ID,
		Slug:       session.Slug,
		ACPAgent:   "codex",
		ACPSession: "acp-session",
		State:      acp.StateIdle,
		ToolCalls:  []sessionevents.ACPToolCall{heavyCall},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	req.Header.Set("X-Jaz-Client-Platform", "mobile")
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, forbidden := range []string{
		"very large tool result",
		"expensive command input",
		"activity entry not rendered",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("mobile response contains stripped payload %q: %s", forbidden, body)
		}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(res.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["activity"]; ok {
		t.Fatalf("mobile response includes activity: %s", raw["activity"])
	}
	var got struct {
		Events       []sessionevents.Event       `json:"events"`
		ACPToolCalls []sessionevents.ACPToolCall `json:"acp_tool_calls"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || got.Events[0].ACP == nil || len(got.Events[0].ACP.ToolCalls) != 1 {
		t.Fatalf("events = %#v", got.Events)
	}
	eventCall := got.Events[0].ACP.ToolCalls[0]
	if eventCall.ID != heavyCall.ID || eventCall.Title != heavyCall.Title || eventCall.Status != heavyCall.Status {
		t.Fatalf("event tool call summary = %#v", eventCall)
	}
	if len(eventCall.Content) != 0 || len(eventCall.RawInput) != 0 || !eventCall.Runtime.IsZero() || eventCall.Kind != "" || eventCall.ToolName != "" {
		t.Fatalf("event tool call retained heavy fields: %#v", eventCall)
	}
	if len(got.ACPToolCalls) != 1 {
		t.Fatalf("acp_tool_calls = %#v", got.ACPToolCalls)
	}
	stateCall := got.ACPToolCalls[0]
	if stateCall.ID != heavyCall.ID || stateCall.Title != heavyCall.Title || stateCall.Status != heavyCall.Status {
		t.Fatalf("state tool call summary = %#v", stateCall)
	}
	if len(stateCall.Content) != 0 || len(stateCall.RawInput) != 0 || !stateCall.Runtime.IsZero() || stateCall.Kind != "" || stateCall.ToolName != "" {
		t.Fatalf("state tool call retained heavy fields: %#v", stateCall)
	}
}

func TestSessionMessagesMobilePreservesPermissionContent(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "claude-plan",
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
	permission := sessionevents.ACPPermission{
		ID:         "perm-1",
		Title:      "Ready to code?",
		ToolCallID: "toolu-plan-exit",
		Content:    "1. Inspect the provider path.\n2. Add the flag.",
		Status:     "pending",
		Options: []sessionevents.ACPPermissionOption{{
			ID:   "allow",
			Name: "Allow",
		}},
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type:       "permission_request",
		Permission: &permission,
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	req.Header.Set("X-Jaz-Client-Platform", "mobile")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: &fakeACPManager{job: acp.Job{
		ID:          session.ID,
		Slug:        session.Slug,
		ACPAgent:    "claude",
		ACPSession:  "acp-session",
		State:       acp.StateRunning,
		Permissions: []sessionevents.ACPPermission{permission},
	}}}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Events         []sessionevents.Event         `json:"events"`
		ACPPermissions []sessionevents.ACPPermission `json:"acp_permissions"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || got.Events[0].Permission == nil {
		t.Fatalf("events = %#v", got.Events)
	}
	if got.Events[0].Permission.Content != permission.Content {
		t.Fatalf("event permission content = %q, want %q", got.Events[0].Permission.Content, permission.Content)
	}
	if len(got.ACPPermissions) != 1 || got.ACPPermissions[0].Content != permission.Content {
		t.Fatalf("acp permissions = %#v", got.ACPPermissions)
	}
}

func TestSessionMessagesIncludesActiveOperation(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-compacting",
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

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: &fakeACPManager{job: acp.Job{
		ID:              session.ID,
		Slug:            session.Slug,
		ACPAgent:        "codex",
		ACPSession:      "acp-session",
		State:           acp.StateRunning,
		GoalRequested:   true,
		ActiveOperation: acp.ActiveOperationCompact,
	}}}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ActiveOperation string `json:"acp_active_operation"`
		GoalRequested   bool   `json:"acp_goal_requested"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ActiveOperation != acp.ActiveOperationCompact {
		t.Fatalf("active operation = %q, want compact", got.ActiveOperation)
	}
	if !got.GoalRequested {
		t.Fatalf("goal requested = false, want true")
	}
}

func TestSessionMessagesErrorChildPreservesParentVisibility(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{
		Slug:    "parent",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{
		Slug:     "child",
		Title:    "Child task",
		ParentID: parent.ID,
		Runtime:  storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveACPState(child.ID, storage.ACPState{
		ID:            child.ID,
		Slug:          child.Slug,
		Title:         child.Title,
		ParentID:      parent.ID,
		ACPAgent:      "codex",
		ACPSession:    "acp-session",
		State:         acp.StateRunning,
		ParentVisible: true,
		Assistant:     "private child output",
		Thought:       "private child thought",
		Plan:          []sessionevents.ACPPlanEntry{{Content: "Inspect current page", Status: "completed"}},
		ToolCalls:     []sessionevents.ACPToolCall{{ID: "tool-1", Title: "read file"}},
		Permissions: []sessionevents.ACPPermission{{
			ID:     "perm-1",
			Status: "pending",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	child, err = store.LoadSession(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	child.Status = storage.StatusError
	child.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(child); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+parent.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: &fakeACPManager{
		job: acp.Job{State: acp.StateNotRunning},
		jobs: []acp.Job{{
			ID:            child.ID,
			Slug:          child.Slug,
			Title:         child.Title,
			ParentID:      parent.ID,
			ACPAgent:      "codex",
			ACPSession:    "acp-session",
			State:         acp.StateRunning,
			ParentVisible: true,
		}},
	}}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACPChildren []storage.ACPState `json:"acp_children"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ACPChildren) != 1 {
		t.Fatalf("children = %#v", got.ACPChildren)
	}
	state := got.ACPChildren[0]
	if state.ID != child.ID || state.State != acp.StateFailed || state.Error != child.Error {
		t.Fatalf("child state = %#v", state)
	}
	if !state.ParentVisible {
		t.Fatalf("parent_visible = false, want true")
	}
	if state.Assistant != "" || state.Thought != "" || len(state.Plan) != 0 || len(state.ToolCalls) != 0 || len(state.Permissions) != 0 {
		t.Fatalf("child transcript leaked into parent response: %#v", state)
	}
}
