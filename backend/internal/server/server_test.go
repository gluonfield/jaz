package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
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

func TestCreateACPSessionForwardsWorktree(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{spawnStore: store}

	body := `{"runtime":"acp","agent":"codex","directory":"repo","worktree":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.spawned.Directory != "repo" || !manager.spawned.Worktree {
		t.Fatalf("spawn request = %#v, want Directory=repo Worktree=true", manager.spawned)
	}
}

func TestCreateACPSessionForwardsModelOverride(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{spawnStore: store}

	body := `{"runtime":"acp","agent":"claude","model":"sonnet"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.spawned.Model != "sonnet" {
		t.Fatalf("spawn request = %#v, want Model=sonnet", manager.spawned)
	}
}

func TestLegacyClaudeACPAgentCanonicalizedInSessionResponses(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacyClaudeName := strings.ReplaceAll("claude-code", "-", "_")
	legacy, err := store.CreateSession(storage.CreateSession{
		Slug:          "legacy-claude",
		Runtime:       storage.RuntimeACP,
		ModelProvider: legacyClaudeName,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     legacyClaudeName,
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}
	var listed struct {
		Sessions []storage.Session `json:"sessions"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].RuntimeRef == nil ||
		listed.Sessions[0].RuntimeRef.Agent != acp.AgentClaude ||
		listed.Sessions[0].ModelProvider != acp.AgentClaude {
		t.Fatalf("listed sessions = %#v", listed.Sessions)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+legacy.ID, nil)
	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
	var got storage.Session
	if err := json.Unmarshal(getRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RuntimeRef == nil || got.RuntimeRef.Agent != acp.AgentClaude || got.ModelProvider != acp.AgentClaude {
		t.Fatalf("session = %#v", got)
	}

	parent, err := store.CreateSession(storage.CreateSession{Slug: "parent", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{
		Slug:     "legacy-child",
		ParentID: parent.ID,
		Runtime:  storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     legacyClaudeName,
			SessionID: "child-acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveACPState(child.ID, storage.ACPState{
		ID:            child.ID,
		Slug:          child.Slug,
		ParentID:      parent.ID,
		ACPAgent:      legacyClaudeName,
		ACPSession:    "child-acp-session",
		State:         acp.StateIdle,
		ParentVisible: true,
	}); err != nil {
		t.Fatal(err)
	}
	messagesReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+parent.ID+"/messages", nil)
	messagesRes := httptest.NewRecorder()
	handler.ServeHTTP(messagesRes, messagesReq)
	if messagesRes.Code != http.StatusOK {
		t.Fatalf("messages status = %d, body = %s", messagesRes.Code, messagesRes.Body.String())
	}
	var messages struct {
		ACPChildren []storage.ACPState `json:"acp_children"`
	}
	if err := json.Unmarshal(messagesRes.Body.Bytes(), &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages.ACPChildren) != 1 || messages.ACPChildren[0].ACPAgent != acp.AgentClaude {
		t.Fatalf("acp children = %#v", messages.ACPChildren)
	}
}

func TestListWorkspaceDirsReportsGit(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"repo", "plain"} {
		if err := os.Mkdir(filepath.Join(workspace, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(workspace, "repo", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/workspace/dirs?path=", nil)
	res := httptest.NewRecorder()
	(&Server{Workspace: workspace}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Path string `json:"path"`
		Git  bool   `json:"git"`
		Dirs []struct {
			Name string `json:"name"`
			Git  bool   `json:"git"`
		} `json:"dirs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Dirs) != 2 {
		t.Fatalf("dirs = %#v, want 2 entries", got.Dirs)
	}
	want := map[string]bool{"repo": true, "plain": false}
	for _, dir := range got.Dirs {
		if got, ok := want[dir.Name]; !ok || got != dir.Git {
			t.Fatalf("dir %q git = %v, want %v", dir.Name, dir.Git, want[dir.Name])
		}
	}
}

func TestProjectRoutesPersistServerDirectories(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "jaz")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()

	rawDir, err := json.Marshal(dir)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"path":` + string(rawDir) + `}`
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("duplicate status = %d, body = %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Projects []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Git  bool   `json:"git"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Projects) != 1 || got.Projects[0].Name != "jaz" || got.Projects[0].Path != dir || !got.Projects[0].Git {
		t.Fatalf("projects = %#v, want jaz %q git", got.Projects, dir)
	}
}

func TestListFilesystemDirsReportsServerPaths(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/filesystem/dirs?path="+url.QueryEscape(root), nil)
	res := httptest.NewRecorder()
	(&Server{}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Path   string `json:"path"`
		Parent string `json:"parent"`
		Dirs   []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Git  bool   `json:"git"`
		} `json:"dirs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != root || got.Parent != filepath.Dir(root) {
		t.Fatalf("path = %q parent = %q, want %q / %q", got.Path, got.Parent, root, filepath.Dir(root))
	}
	if len(got.Dirs) != 1 || got.Dirs[0].Name != "repo" || got.Dirs[0].Path != repo || !got.Dirs[0].Git {
		t.Fatalf("dirs = %#v, want repo %q git", got.Dirs, repo)
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
		Session:   session,
	}, nil
}

func (f *fakeACPManager) Agents() []string { return nil }

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
