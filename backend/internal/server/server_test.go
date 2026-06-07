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
	"github.com/wins/jaz/backend/internal/agent"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestACPBackedSessionRoutesToACPManager(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-whoami",
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
	manager := &fakeACPManager{job: acp.Job{
		ID:        session.ID,
		Slug:      session.Slug,
		State:     acp.StateIdle,
		Assistant: "codex says hi",
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/codex-whoami/messages:stream", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.sent.Session != session.ID || manager.sent.Message != "hi" {
		t.Fatalf("unexpected send request %#v", manager.sent)
	}
	if !strings.Contains(res.Body.String(), "codex says hi") {
		t.Fatalf("missing acp assistant output: %s", res.Body.String())
	}
}

func TestSessionMessagesIncludesPersistedACPChildren(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{
		Slug:    "parent",
		Runtime: storage.RuntimeNative,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{
		Slug:     "codex-plan",
		Title:    "Codex plan",
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
		State:         acp.StateIdle,
		ParentVisible: true,
		Plan:          []sessionevents.ACPPlanEntry{{Content: "Inspect current page", Status: "completed"}},
		Permissions: []sessionevents.ACPPermission{{
			ID:     "perm-1",
			Title:  "Clarifying questions",
			Status: "pending",
			Questions: []sessionevents.ACPQuestion{{
				ID:       "audience",
				Question: "Who is the page for?",
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+parent.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

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
	childState := got.ACPChildren[0]
	if len(childState.Plan) != 1 || childState.Plan[0].Content != "Inspect current page" {
		t.Fatalf("plan = %#v", childState.Plan)
	}
	if len(childState.Permissions) != 1 || len(childState.Permissions[0].Questions) != 1 {
		t.Fatalf("permissions = %#v", childState.Permissions)
	}
}

func TestSessionMessagesIncludesPersistedSessionEvents(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex",
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
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type:    "acp_message",
		Content: "I inspected the workspace.",
		ACP: &sessionevents.ACPEvent{
			ID:        session.ID,
			Slug:      session.Slug,
			Agent:     "codex",
			SessionID: "acp-session",
			State:     acp.StateRunning,
		},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Events []sessionevents.Event `json:"events"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || got.Events[0].Type != "acp_message" || got.Events[0].Content != "I inspected the workspace." {
		t.Fatalf("events = %#v", got.Events)
	}
}

func TestSessionMessagesHidesDirectACPChildStateFromParent(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{Slug: "parent", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{
		Slug:     "codex-direct",
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
		ID:         child.ID,
		Slug:       child.Slug,
		ParentID:   parent.ID,
		ACPAgent:   "codex",
		ACPSession: "acp-session",
		State:      acp.StateIdle,
		Assistant:  "hello from direct child chat",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+parent.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACPChildren []storage.ACPState `json:"acp_children"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ACPChildren) != 0 {
		t.Fatalf("children = %#v", got.ACPChildren)
	}
}

type fakeACPManager struct {
	sent acp.SendRequest
	job  acp.Job
}

func (f *fakeACPManager) Send(_ context.Context, req acp.SendRequest) (acp.Job, error) {
	f.sent = req
	return f.job, nil
}

func (f *fakeACPManager) Status(string) (acp.Job, error) {
	return f.job, nil
}

func (f *fakeACPManager) List() []acp.Job {
	return []acp.Job{f.job}
}

func (f *fakeACPManager) AnswerInteractive(context.Context, acp.InteractiveAnswer) error {
	return nil
}

func (f *fakeACPManager) Cancel(context.Context, string) (acp.Job, error) {
	return f.job, nil
}

type slowTool struct{ delay time.Duration }

func (s *slowTool) Definition() tools.Definition {
	return tools.Function("exec_command", "stub", true, map[string]any{"type": "object"})
}

func (s *slowTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	time.Sleep(s.delay)
	return tools.Result{Content: `{"status":"ok"}`}, nil
}

// The transcript interleaves messages with session events by timestamp, so
// each row must be stamped when it was produced: the user message at turn
// start and each assistant round before its tools run.
func TestNativeStreamStampsRowsChronologically(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "native", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	delay := 60 * time.Millisecond
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{
			Provider: mockprovider.New(),
			Tools:    tools.NewRegistry(&slowTool{delay: delay}),
			MaxTurns: 4,
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"run the mock"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want user + 2 assistant rounds: %#v", len(records), records)
	}
	if records[0].Role != "user" || records[1].Role != "assistant" || records[2].Role != "assistant" {
		t.Fatalf("unexpected roles: %s %s %s", records[0].Role, records[1].Role, records[2].Role)
	}
	for i := 1; i < len(records); i++ {
		if records[i].CreatedAt.Before(records[i-1].CreatedAt) {
			t.Fatalf("row %d stamped before row %d: %v >= %v", i+1, i, records[i-1].CreatedAt, records[i].CreatedAt)
		}
	}
	// The tool round is stamped before its tool executes; the final round after.
	gap := records[2].CreatedAt.Sub(records[1].CreatedAt)
	if gap < delay-10*time.Millisecond {
		t.Fatalf("tool round was not stamped before tool execution: gap %v, want >= %v", gap, delay)
	}
	var toolBlock *storage.Block
	for i := range records[1].Blocks {
		if records[1].Blocks[i].Type == "tool" {
			toolBlock = &records[1].Blocks[i]
		}
	}
	if toolBlock == nil || toolBlock.Result != `{"status":"ok"}` {
		t.Fatalf("tool block missing or unresolved: %#v", records[1].Blocks)
	}
}
