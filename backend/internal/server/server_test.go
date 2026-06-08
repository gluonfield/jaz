package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/provider"
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

func TestMCPServerSettingsAPI(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store}).Handler()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{
		"name":"Docs",
		"url":"https://mcp.example.com/mcp",
		"enabled":true,
		"bearer_token_env_var":"DOCS_TOKEN",
		"headers":[{"name":"X-Team","value":"platform"}],
		"env_headers":[{"name":"X-Secret","env_var":"DOCS_SECRET"}]
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		URL               string `json:"url"`
		Enabled           bool   `json:"enabled"`
		BearerTokenEnvVar string `json:"bearer_token_env_var"`
		Status            string `json:"status"`
		ToolCount         int    `json:"tool_count"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "Docs" || created.URL != "https://mcp.example.com/mcp" ||
		!created.Enabled || created.BearerTokenEnvVar != "DOCS_TOKEN" {
		t.Fatalf("created = %#v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/mcp/servers", nil)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}
	var listed struct {
		Servers []struct {
			ID      string `json:"id"`
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
			EnvHeaders []struct {
				Name   string `json:"name"`
				EnvVar string `json:"env_var"`
			} `json:"env_headers"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Servers) != 1 || listed.Servers[0].ID != created.ID ||
		len(listed.Servers[0].Headers) != 1 || listed.Servers[0].Headers[0].Value != "platform" ||
		len(listed.Servers[0].EnvHeaders) != 1 || listed.Servers[0].EnvHeaders[0].EnvVar != "DOCS_SECRET" {
		t.Fatalf("listed = %#v", listed)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/"+created.ID+"/disable", nil)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, disableReq)
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	loaded, err := store.LoadMCPServer(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Enabled {
		t.Fatalf("server still enabled: %#v", loaded)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/mcp/servers/"+created.ID, nil)
	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}
	servers, err := store.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers after delete = %#v", servers)
	}
}

// The renderer is a separate origin, so a DELETE is preflighted. The handler
// must advertise DELETE in Access-Control-Allow-Methods or the browser blocks
// the request before it ever reaches us.
func TestCORSAllowsDeletePreflight(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()

	req := httptest.NewRequest(http.MethodOptions, "/v1/mcp/servers/abc", nil)
	req.Header.Set("Origin", "http://localhost:5180")
	req.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d", res.Code)
	}
	if allow := res.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(allow, http.MethodDelete) {
		t.Fatalf("Access-Control-Allow-Methods = %q, missing DELETE", allow)
	}
}

type blockingMCPRuntime struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingMCPRuntime) Refresh(ctx context.Context) {
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
	case <-ctx.Done():
	}
}

func (r *blockingMCPRuntime) Status(string) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *blockingMCPRuntime) Test(context.Context, mcpconfig.Server) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *blockingMCPRuntime) Authorize(context.Context, mcpconfig.Server) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func TestMCPServerSettingsRefreshDoesNotBlockResponse(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runtime := &blockingMCPRuntime{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(runtime.release)
	handler := (&Server{Store: store, MCP: runtime}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{
		"name":"Docs",
		"url":"https://mcp.example.com/mcp",
		"enabled":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", res.Code, res.Body.String())
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("response waited on refresh for %s", elapsed)
	}
	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("refresh was not scheduled")
	}
}

func TestMCPServerSettingsRejectInvalidURL(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{
		"name":"Docs",
		"url":"ftp://mcp.example.com/mcp",
		"enabled":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400, body = %s", res.Code, res.Body.String())
	}
}

func TestACPStreamUsesServerContextAfterRequestCancel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-detached-stream",
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
		Assistant: "done",
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"hi"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	sendCtxErr := manager.sendCtxErr
	manager.mu.Unlock()
	if sendCtxErr != nil {
		t.Fatalf("acp send used cancelled request context: %v", sendCtxErr)
	}
}

func TestACPInteractiveResponseUsesServerContextAfterRequestCancel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-detached-interactive",
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
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateIdle,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/interactive-response", strings.NewReader(`{"text":"continue"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	answerCtxErr := manager.answerCtxErr
	manager.mu.Unlock()
	if answerCtxErr != nil {
		t.Fatalf("interactive response used cancelled request context: %v", answerCtxErr)
	}
}

func TestACPCancelUsesServerContextAfterRequestCancel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-detached-cancel",
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
	session.Status = storage.StatusRunning
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:    session.ID,
		Slug:  session.Slug,
		State: acp.StateRunning,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/cancel", nil).WithContext(ctx)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	cancelCtxErr := manager.cancelCtxErr
	manager.mu.Unlock()
	if cancelCtxErr != nil {
		t.Fatalf("cancel used cancelled request context: %v", cancelCtxErr)
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
	mu           sync.Mutex
	sent         acp.SendRequest
	answered     acp.InteractiveAnswer
	sendCtxErr   error
	answerCtxErr error
	cancelCtxErr error
	sendErr      error
	answerErr    error
	job          acp.Job
	spawnStore   storage.SessionStore
	spawned      acp.SpawnRequest
	spawnErr     error
}

func (f *fakeACPManager) Spawn(_ context.Context, req acp.SpawnRequest) (acp.SpawnResult, error) {
	f.mu.Lock()
	f.spawned = req
	spawnStore := f.spawnStore
	spawnErr := f.spawnErr
	f.mu.Unlock()
	if spawnErr != nil {
		return acp.SpawnResult{}, spawnErr
	}
	if spawnStore == nil {
		return acp.SpawnResult{}, nil
	}
	session, err := spawnStore.CreateSession(storage.CreateSession{
		Slug:       req.Slug,
		Title:      req.Title,
		Runtime:    storage.RuntimeACP,
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     req.ACPAgent,
			SessionID: "fake-acp-session",
		},
	})
	if err != nil {
		return acp.SpawnResult{}, err
	}
	return acp.SpawnResult{
		Status:    "created",
		SessionID: session.ID,
		Slug:      session.Slug,
		ACPAgent:  req.ACPAgent,
		State:     acp.StateIdle,
	}, nil
}

func (f *fakeACPManager) Send(ctx context.Context, req acp.SendRequest) (acp.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = req
	f.sendCtxErr = ctx.Err()
	return f.job, f.sendErr
}

func (f *fakeACPManager) Status(string) (acp.Job, error) {
	return f.job, nil
}

func (f *fakeACPManager) List() []acp.Job {
	return []acp.Job{f.job}
}

func (f *fakeACPManager) AnswerInteractive(ctx context.Context, answer acp.InteractiveAnswer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = answer
	f.answerCtxErr = ctx.Err()
	return f.answerErr
}

func (f *fakeACPManager) Cancel(ctx context.Context, _ string) (acp.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCtxErr = ctx.Err()
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

type blockingProvider struct {
	started     chan struct{}
	startedOnce sync.Once
	release     chan struct{}
}

func (p *blockingProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *blockingProvider) StreamComplete(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 2)
	go func() {
		defer close(ch)
		p.startedOnce.Do(func() { close(p.started) })
		select {
		case <-p.release:
			ch <- provider.Event{Type: provider.EventDelta, Delta: "done"}
			ch <- provider.Event{Type: provider.EventDone}
		case <-ctx.Done():
			ch <- provider.Event{Type: provider.EventError, Err: ctx.Err()}
		}
	}()
	return ch, nil
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
