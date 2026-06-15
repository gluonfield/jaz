package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	"github.com/wins/jaz/backend/internal/sessionevents"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	plantool "github.com/wins/jaz/backend/internal/tools/plan"
)

type slowTool struct{ delay time.Duration }

func (s *slowTool) Definition() tools.Definition {
	return tools.Function("exec_command", "stub", true, map[string]any{"type": "object"})
}

func (s *slowTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	time.Sleep(s.delay)
	return tools.Result{Content: `{"status":"ok"}`}, nil
}

type requestRecorderProvider struct {
	requests []provider.Request
}

func (p *requestRecorderProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

type planToolProvider struct {
	requests []provider.Request
}

func (p *planToolProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *planToolProvider) StreamComplete(_ context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.requests = append(p.requests, req)
	ch := make(chan provider.Event, 2)
	if len(p.requests) == 1 {
		call := provider.FunctionToolCall("call_plan", "update_plan", `{"explanation":"Ready for approval.","plan":[{"step":"Inspect files","status":"pending"},{"step":"Confirm approach","status":"pending"}]}`)
		ch <- provider.Event{Type: provider.EventToolCall, ToolCall: &call}
	} else {
		ch <- provider.Event{Type: provider.EventDelta, Delta: "Plan ready for approval."}
	}
	ch <- provider.Event{Type: provider.EventDone}
	close(ch)
	return ch, nil
}

func (p *requestRecorderProvider) StreamComplete(_ context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.requests = append(p.requests, req)
	ch := make(chan provider.Event, 2)
	ch <- provider.Event{Type: provider.EventDelta, Delta: "done"}
	ch <- provider.Event{Type: provider.EventDone}
	close(ch)
	return ch, nil
}

type titleProvider struct {
	mu             sync.Mutex
	titleRequests  []provider.Request
	streamRequests []provider.Request
}

func (p *titleProvider) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	if req.StructuredOutput != nil {
		p.mu.Lock()
		p.titleRequests = append(p.titleRequests, req)
		p.mu.Unlock()
		return provider.Response{Message: provider.AssistantMessage(`{"title":"Fix login redirect"}`, nil)}, nil
	}
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *titleProvider) StreamComplete(_ context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.streamRequests = append(p.streamRequests, req)
	p.mu.Unlock()
	ch := make(chan provider.Event, 2)
	ch <- provider.Event{Type: provider.EventDelta, Delta: "done"}
	ch <- provider.Event{Type: provider.EventDone}
	close(ch)
	return ch, nil
}

func (p *titleProvider) counts() (int, int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	structured := len(p.titleRequests) == 1 && p.titleRequests[0].StructuredOutput != nil
	return len(p.titleRequests), len(p.streamRequests), structured
}

type blockingTitleProvider struct {
	streamStarted chan struct{}
	releaseTitle  chan struct{}
}

func (p *blockingTitleProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if req.StructuredOutput == nil {
		return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
	}
	select {
	case <-p.releaseTitle:
		return provider.Response{Message: provider.AssistantMessage(`{"title":"Async title"}`, nil)}, nil
	case <-ctx.Done():
		return provider.Response{}, ctx.Err()
	}
}

func (p *blockingTitleProvider) StreamComplete(context.Context, provider.Request) (<-chan provider.Event, error) {
	closeOnce(p.streamStarted)
	ch := make(chan provider.Event, 2)
	ch <- provider.Event{Type: provider.EventDelta, Delta: "done"}
	ch <- provider.Event{Type: provider.EventDone}
	close(ch)
	return ch, nil
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func TestNativeTurnUsesStoredProviderModelAndReasoning(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:            "native-provider",
		Runtime:         storage.RuntimeNative,
		ModelProvider:   "openai",
		Model:           "gpt-test",
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorderProvider{}
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{Provider: recorder, MaxTurns: 1},
	}

	if status := srv.runNativeSession(context.Background(), session, "hello", false, nil); status != storage.StatusIdle {
		t.Fatalf("status = %s", status)
	}
	if len(recorder.requests) != 1 {
		t.Fatalf("requests = %#v", recorder.requests)
	}
	req := recorder.requests[0]
	if req.Provider != "openai" || req.Model != "gpt-test" || req.ReasoningEffort != "high" {
		t.Fatalf("unexpected provider request %#v", req)
	}
}

func TestBeginNativeTurnClearsStaleError(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:          "native-retry",
		Runtime:       storage.RuntimeNative,
		ModelProvider: "openai",
		Model:         "gpt-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusError
	session.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, status, _, err := (&Server{Store: store, Events: events}).beginNativeTurn(context.Background(), session, "continue", false)
	if err != nil {
		t.Fatal(err)
	}
	if status != storage.StatusRunning {
		t.Fatalf("status = %s, want %s", status, storage.StatusRunning)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusRunning || loaded.Error != "" {
		t.Fatalf("loaded status/error = %q/%q, want running with no error", loaded.Status, loaded.Error)
	}
	expectSessionChangedEvent(t, sub, session.ID)
}

func expectSessionChangedEvent(t *testing.T, sub <-chan sessionevents.Event, sessionID string) {
	t.Helper()
	select {
	case event := <-sub:
		if event.SessionID != sessionID || event.Type != sessionevents.TypeSession {
			t.Fatalf("event = %#v, want session change for %s", event, sessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session change event")
	}
}

func TestNativeTurnFailsWhenPromptCannotBuild(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-prompt-error", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorderProvider{}
	srv := &Server{
		Store:   store,
		Agent:   &agent.Agent{Provider: recorder, MaxTurns: 1},
		Prompts: coordinator.NewBuilder(root, "", "", nil),
	}

	if status := srv.runNativeSession(context.Background(), session, "hello", false, nil); status != storage.StatusError {
		t.Fatalf("status = %s, want %s", status, storage.StatusError)
	}
	if len(recorder.requests) != 0 {
		t.Fatalf("provider was called after prompt build failure: %#v", recorder.requests)
	}
}

func TestNativeFirstTurnGeneratesStructuredTitle(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	message := "please fix the login redirect after OAuth callback"
	session, err := store.CreateSession(storage.CreateSession{
		Slug:          "oauth-redirect",
		Title:         message,
		Runtime:       storage.RuntimeNative,
		ModelProvider: "openai",
		Model:         "gpt-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &titleProvider{}
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{Provider: provider, MaxTurns: 1},
	}

	if status := srv.runNativeSession(context.Background(), session, message, false, nil); status != storage.StatusIdle {
		t.Fatalf("status = %s", status)
	}
	deadline := time.Now().Add(time.Second)
	for {
		titleRequests, streamRequests, structured := provider.counts()
		loaded, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if titleRequests == 1 && streamRequests == 1 && structured && loaded.Title == "Fix login redirect" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("title requests = %d, stream requests = %d, structured = %v, title = %q", titleRequests, streamRequests, structured, loaded.Title)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNativeFirstTurnDoesNotWaitForStructuredTitle(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	message := "please fix the login redirect after OAuth callback"
	session, err := store.CreateSession(storage.CreateSession{
		Slug:          "oauth-redirect",
		Title:         message,
		Runtime:       storage.RuntimeNative,
		ModelProvider: "openai",
		Model:         "gpt-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &blockingTitleProvider{
		streamStarted: make(chan struct{}),
		releaseTitle:  make(chan struct{}),
	}
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{Provider: provider, MaxTurns: 1},
	}

	done := make(chan string, 1)
	go func() {
		done <- srv.runNativeSession(context.Background(), session, message, false, nil)
	}()

	select {
	case <-provider.streamStarted:
	case <-time.After(200 * time.Millisecond):
		closeOnce(provider.releaseTitle)
		t.Fatal("native stream waited for title generation")
	}
	closeOnce(provider.releaseTitle)

	select {
	case status := <-done:
		if status != storage.StatusIdle {
			t.Fatalf("status = %s", status)
		}
	case <-time.After(time.Second):
		t.Fatal("native turn did not finish")
	}
}

func TestCreateNativeSessionAppliesModelOverrides(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store}).Handler()

	post := func(body string) (*httptest.ResponseRecorder, storage.Session) {
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		var session storage.Session
		if res.Code == http.StatusOK {
			if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
				t.Fatal(err)
			}
		}
		return res, session
	}

	res, session := post(`{"model_provider":"openai","model":"gpt-5.5","reasoning_effort":"xhigh"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if session.ModelProvider != "openai" || session.Model != "gpt-5.5" || session.ReasoningEffort != "xhigh" {
		t.Fatalf("session = %#v, want openai/gpt-5.5/xhigh", session)
	}

	// Switching providers without naming a model falls back to that provider's
	// default model rather than keeping the other provider's default.
	res, session = post(`{"model_provider":"openai"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if session.ModelProvider != "openai" || session.Model != "gpt-5.4-mini" {
		t.Fatalf("session = %#v, want openai/gpt-5.4-mini", session)
	}

	res, _ = post(`{"model_provider":"bogus"}`)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "unknown native provider") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestCreateNativeSessionPersistsWorkingDirectory(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	workspace := t.TempDir()
	handler := (&Server{Store: store, Workspace: workspace}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var session storage.Session
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.Type != storage.RuntimeNative ||
		session.RuntimeRef.Cwd != workspace || session.RuntimeRef.ProjectPath != "" {
		t.Fatalf("default runtime ref = %#v, want native cwd %q", session.RuntimeRef, workspace)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RuntimeRef == nil || loaded.RuntimeRef.Type != storage.RuntimeNative ||
		loaded.RuntimeRef.Cwd != workspace || loaded.RuntimeRef.ProjectPath != "" {
		t.Fatalf("loaded default runtime ref = %#v, want native cwd %q", loaded.RuntimeRef, workspace)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"worktree":true}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "worktree requires") {
		t.Fatalf("worktree default status = %d, body = %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"directory":"repo"}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("directory status = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(workspace, "repo")
	if session.RuntimeRef == nil || session.RuntimeRef.Type != storage.RuntimeNative ||
		session.RuntimeRef.Cwd != want || session.RuntimeRef.ProjectPath != want {
		t.Fatalf("runtime ref = %#v, want native cwd %q", session.RuntimeRef, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("working directory was not created: %v", err)
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RuntimeRef == nil || loaded.RuntimeRef.Type != storage.RuntimeNative ||
		loaded.RuntimeRef.Cwd != want || loaded.RuntimeRef.ProjectPath != want {
		t.Fatalf("loaded runtime ref = %#v, want native cwd %q", loaded.RuntimeRef, want)
	}

	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{"directory": project})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("absolute status = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.Type != storage.RuntimeNative ||
		session.RuntimeRef.Cwd != project || session.RuntimeRef.ProjectPath != project {
		t.Fatalf("absolute runtime ref = %#v, want native cwd %q", session.RuntimeRef, project)
	}

	repo := filepath.Join(workspace, "worktree-repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "--version").Run(); err != nil {
		t.Skip("git is not available")
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@jaz"},
		{"config", "user.name", "jaz"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"directory":"worktree-repo","worktree":true}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("worktree status = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	wantWorktree := filepath.Join(workspace, ".worktrees", session.Slug)
	wantProject, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil ||
		session.RuntimeRef.Cwd != wantWorktree ||
		session.RuntimeRef.ProjectPath != wantProject {
		t.Fatalf("worktree runtime ref = %#v, want cwd %q project %q", session.RuntimeRef, wantWorktree, wantProject)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"directory":"../outside"}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "escapes") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestCreateNativeWorktreeFromWorkspaceRootPersistsProjectPath(t *testing.T) {
	if err := exec.Command("git", "--version").Run(); err != nil {
		t.Skip("git is not available")
	}
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	workspace := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@jaz"},
		{"config", "user.name", "jaz"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", workspace}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	handler := (&Server{Store: store, Workspace: workspace}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"directory":".","worktree":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var session storage.Session
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	wantWorktree := filepath.Join(workspace, ".worktrees", session.Slug)
	wantProject, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil ||
		session.RuntimeRef.Cwd != wantWorktree ||
		session.RuntimeRef.ProjectPath != wantProject {
		t.Fatalf("worktree runtime ref = %#v, want cwd %q project %q", session.RuntimeRef, wantWorktree, wantProject)
	}
}

func TestCreateNativeSessionErrorsWhenStoredDefaultsAreIncomplete(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{
		Native: agentsettings.NativeAgentDefaults{Model: "gpt-test"},
		ACP:    map[string]agentsettings.ACPAgentDefaults{},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), "native provider is required") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestCreateNativeSessionErrorsWhenStoredProviderIsUnknown(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{
		Native: agentsettings.NativeAgentDefaults{
			ModelProvider: "missing",
			Model:         "gpt-test",
		},
		ACP: map[string]agentsettings.ACPAgentDefaults{},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), "unknown native provider") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestNativeStreamSendsAttachmentLinksAndKeepsTranscriptBlocks(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-attachments", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorderProvider{}
	handler := (&Server{
		Store:     store,
		Workspace: t.TempDir(),
		Agent:     &agent.Agent{Provider: recorder, MaxTurns: 1},
	}).Handler()
	attachment := uploadTestAttachment(t, handler, session.ID, "note.txt", "read me")

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"summarize","attachment_ids":["`+attachment.ID+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if len(recorder.requests) != 1 {
		t.Fatalf("requests = %#v", recorder.requests)
	}
	requestMessages := recorder.requests[0].Messages
	gotPrompt := provider.MessageContent(requestMessages[len(requestMessages)-1])
	if !strings.Contains(gotPrompt, "summarize\n\nAttachments:\n- note.txt: file://") {
		t.Fatalf("native prompt = %q", gotPrompt)
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Role != "user" || records[0].Content != "summarize" {
		t.Fatalf("records = %#v", records)
	}
	var found bool
	for _, block := range records[0].Blocks {
		if block.Type == "attachment" && block.ID == attachment.ID && block.URI == attachment.URI {
			found = true
		}
		if block.Type == "text" && strings.Contains(block.Text, "Attachments:") {
			t.Fatalf("transcript text leaked attachment prompt: %#v", block)
		}
	}
	if !found {
		t.Fatalf("attachment block not persisted: %#v", records[0].Blocks)
	}
}

func TestNativePlanRequestCreatesProposedPlanApprovalEvent(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-plan", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &planToolProvider{}
	handler := (&Server{
		Store: store,
		Agent: &agent.Agent{
			Provider: recorder,
			Tools:    tools.NewRegistry(&plantool.Tool{Store: store}),
			MaxTurns: 3,
		},
	}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"make a plan","plan_requested":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if len(recorder.requests) != 2 {
		t.Fatalf("requests = %#v", recorder.requests)
	}
	var sawPlanNote, sawPlanTool, sawPlanUserInstruction bool
	for _, msg := range recorder.requests[0].Messages {
		if msg.OfDeveloper != nil && provider.MessageContent(msg) == nativePlanModeNote {
			sawPlanNote = true
		}
		if msg.OfUser != nil && strings.Contains(provider.MessageContent(msg), nativePlanUserInstruction) {
			sawPlanUserInstruction = true
		}
	}
	for _, def := range recorder.requests[0].Tools {
		if tools.DefinitionName(def) == "update_plan" {
			sawPlanTool = true
		}
	}
	if !sawPlanNote || !sawPlanTool || !sawPlanUserInstruction {
		t.Fatalf("sawPlanNote=%v sawPlanTool=%v sawPlanUserInstruction=%v request=%#v", sawPlanNote, sawPlanTool, sawPlanUserInstruction, recorder.requests[0])
	}

	messages, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || provider.MessageContent(messages[0]) != "make a plan" || strings.Contains(provider.MessageContent(messages[0]), nativePlanUserInstruction) {
		t.Fatalf("messages = %#v", messages)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "proposed_plan" || events[0].Plan == nil {
		t.Fatalf("events = %#v", events)
	}
	if !events[0].Plan.AwaitingApproval || events[0].ACP != nil {
		t.Fatalf("plan event = %#v", events[0])
	}
	if events[0].Plan.Explanation != "Ready for approval." || len(events[0].Plan.Plan) != 2 {
		t.Fatalf("plan event = %#v", events[0].Plan)
	}
}

func TestNativeTurnReplaysPersistedToolMediaRefs(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-media-replay", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	blob := []byte("\x89PNG\r\n\x1a\nimage-bytes")
	sum := sha256.Sum256(blob)
	blobPath := filepath.Join(t.TempDir(), "image-blob")
	if err := os.WriteFile(blobPath, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	call := provider.FunctionToolCall("call_view", "view_image", `{"path":"image.png"}`)
	seed := []provider.Message{
		provider.UserMessage("look"),
		provider.AssistantMessage("", []provider.ToolCall{call}),
		provider.ToolMessage(`{"status":"ok","message":"Image attached for visual inspection."}`, "call_view"),
	}
	ref := media.Ref{
		Type:     media.TypeInputImage,
		Text:     "Image returned by view_image: image.png",
		BlobPath: blobPath,
		MimeType: "image/png",
		Size:     int64(len(blob)),
		SHA256:   hex.EncodeToString(sum[:]),
		Detail:   "auto",
		Filename: "image.png",
	}
	if err := store.SaveMessagesWithReasoningAndMedia(session.ID, seed, nil, map[string][]media.Ref{"call_view": []media.Ref{ref}}); err != nil {
		t.Fatal(err)
	}

	recorder := &requestRecorderProvider{}
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{Provider: recorder, MaxTurns: 1},
	}
	if status := srv.runNativeSession(context.Background(), session, "what do you see now?", false, nil); status != storage.StatusIdle {
		t.Fatalf("status = %s", status)
	}
	if len(recorder.requests) != 1 {
		t.Fatalf("requests = %#v", recorder.requests)
	}
	requestJSON, err := json.Marshal(recorder.requests[0].Messages)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(requestJSON), "data:image/png;base64,") {
		t.Fatalf("provider request did not replay image data: %s", requestJSON)
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	refs := storage.MediaRefsByToolCall(records)
	if got := refs["call_view"]; len(got) != 1 || got[0].BlobPath != blobPath {
		t.Fatalf("stored media refs = %#v", refs)
	}
	rawRecords, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawRecords), "data:image") {
		t.Fatalf("stored records leaked base64 image data: %s", rawRecords)
	}
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
