package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/goal"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionview"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
)

type blockingTranscriptStore struct {
	transcript.Store
	started chan struct{}
}

func (s *blockingTranscriptStore) LoadTranscriptPage(ctx context.Context, _ string, _ storage.TranscriptPageRequest) (storage.TranscriptPage, error) {
	close(s.started)
	<-ctx.Done()
	return storage.TranscriptPage{}, ctx.Err()
}

type changedTranscriptStore struct{ transcript.Store }

func (s changedTranscriptStore) LoadTranscriptPage(context.Context, string, storage.TranscriptPageRequest) (storage.TranscriptPage, error) {
	return storage.TranscriptPage{}, storage.ErrTranscriptChanged
}

type noLiveTranscript struct{}

func (noLiveTranscript) HydrationJobs([]string) map[string]acp.HydrationView { return nil }

func sessionMessagesHandler(store storage.Store, reader transcript.Store, manager *fakeACPManager) http.Handler {
	var live transcript.LiveReader = noLiveTranscript{}
	var runtime ACPManager
	if manager != nil {
		live = manager
		runtime = manager
	}
	handler := sessionsapi.NewMessagesHandler(transcript.NewService(reader, live))
	return (&Server{
		Store: store,
		ACP:   runtime,
		Routes: Routes{{
			Pattern: "GET /v1/sessions/{session}/messages",
			Handler: handler,
		}},
	}).Handler()
}

func TestSessionMessagesCancelsTranscriptRead(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "cancel-hydration", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	blocking := &blockingTranscriptStore{Store: store, started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil).WithContext(ctx)
	res := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		sessionMessagesHandler(store, blocking, nil).ServeHTTP(res, req)
		close(done)
	}()
	<-blocking.started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled transcript read did not return")
	}
}

func TestSessionMessagesReportsChangedHistory(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "changed-history"})
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	sessionMessagesHandler(store, changedTranscriptStore{Store: store}, nil).ServeHTTP(
		res,
		httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil),
	)
	if res.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestSessionMessagesErrorSessionOverridesRunningACPSnapshot(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
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
	session.Status = storage.StatusError
	session.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	res := httptest.NewRecorder()

	manager := &fakeACPManager{job: acp.Job{
		ID:         session.ID,
		Slug:       session.Slug,
		ACPAgent:   "codex",
		ACPSession: "acp-session",
		State:      acp.StateRunning,
	}}
	sessionMessagesHandler(store, store, manager).ServeHTTP(res, req)

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
	data, err := json.Marshal(sessionview.Public(storage.Session{
		ID:      "thread-1",
		Runtime: storage.RuntimeACP,
		Goal: &goal.State{
			Identity: goal.Identity{
				ID:        "goal-1",
				ThreadID:  "thread-1",
				Objective: "ship it",
				Status:    goal.StatusActive,
			},
			Budget: goal.Budget{
				TokenBudget: &budget,
				TokensUsed:  25,
			},
			Timestamps: goal.Timestamps{CreatedAt: time.Now().UTC()},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	goalBody := goalField(t, data)
	for _, forbidden := range []string{"created_at", "updated_at"} {
		if strings.Contains(goalBody, forbidden) {
			t.Fatalf("session response leaked goal field %q: %s", forbidden, goalBody)
		}
	}
	for _, required := range []string{`"objective":"ship it"`, `"token_budget":100`, `"tokens_used":25`, `"remaining_tokens":75`} {
		if !strings.Contains(goalBody, required) {
			t.Fatalf("session response missing %s: %s", required, goalBody)
		}
	}
}

func TestSessionEventResponsePublishesPublicGoal(t *testing.T) {
	data, err := json.Marshal(sessionview.Event(sessionevents.Event{
		SessionID: "thread-1",
		Type:      sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{
			Identity: goal.Identity{
				Objective: "ship it",
				Status:    goal.StatusActive,
			},
			Timestamps: goal.Timestamps{CreatedAt: time.Now().UTC()},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	goalBody := goalField(t, data)
	for _, forbidden := range []string{"created_at", "updated_at"} {
		if strings.Contains(goalBody, forbidden) {
			t.Fatalf("event response leaked goal field %q: %s", forbidden, goalBody)
		}
	}
	if !strings.Contains(goalBody, `"objective":"ship it"`) {
		t.Fatalf("event response missing public goal: %s", goalBody)
	}
}

func goalField(t *testing.T, response []byte) string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response, &fields); err != nil {
		t.Fatal(err)
	}
	goalRaw, ok := fields["goal"]
	if !ok {
		t.Fatalf("response missing goal field: %s", response)
	}
	return string(goalRaw)
}

func TestSessionMessagesMobileProjectionStripsHeavyToolPayload(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
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
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	req.Header.Set("X-Jaz-Client-Platform", "mobile")
	res := httptest.NewRecorder()

	sessionMessagesHandler(store, store, nil).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, forbidden := range []string{
		"very large tool result",
		"expensive command input",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("mobile response contains stripped payload %q: %s", forbidden, body)
		}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(res.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
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
	if len(got.ACPToolCalls) != 0 {
		t.Fatalf("inactive snapshot repeated transcript tools: %#v", got.ACPToolCalls)
	}
}

func TestSessionMessagesMobilePreservesPermissionContent(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
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

	manager := &fakeACPManager{job: acp.Job{
		ID:          session.ID,
		Slug:        session.Slug,
		ACPAgent:    "claude",
		ACPSession:  "acp-session",
		State:       acp.StateRunning,
		Permissions: []sessionevents.ACPPermission{permission},
	}}
	sessionMessagesHandler(store, store, manager).ServeHTTP(res, req)

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
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
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

	manager := &fakeACPManager{job: acp.Job{
		ID:              session.ID,
		Slug:            session.Slug,
		ACPAgent:        "codex",
		ACPSession:      "acp-session",
		State:           acp.StateRunning,
		GoalRequested:   true,
		ActiveOperation: acp.ActiveOperationCompact,
	}}
	sessionMessagesHandler(store, store, manager).ServeHTTP(res, req)

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
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
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
	if err := store.AppendSessionEvents(parent.ID, sessionevents.Event{
		Type: "acp",
		ACP:  &sessionevents.ACPEvent{ID: child.ID, Agent: "codex", SessionID: "acp-session", State: acp.StateRunning},
	}); err != nil {
		t.Fatal(err)
	}
	child.Status = storage.StatusError
	child.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(child); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+parent.ID+"/messages", nil)
	res := httptest.NewRecorder()

	manager := &fakeACPManager{
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
	}
	sessionMessagesHandler(store, store, manager).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACPChildren []acp.Job `json:"acp_children"`
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
	if manager.statusCalls != 0 {
		t.Fatalf("status calls = %d, want none", manager.statusCalls)
	}
}
