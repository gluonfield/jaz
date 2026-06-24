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
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestHealthReportsFileReadCapability(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()

	(&Server{}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got healthResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || !got.Capabilities.SessionFileRead {
		t.Fatalf("health = %#v, want ok with session_file_read", got)
	}
}

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

func TestCreateACPSessionCreatesStoredSessionAndForwardsWorktree(t *testing.T) {
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
	if manager.spawned.ACPAgent != "" {
		t.Fatalf("create should defer eager spawn: %#v", manager.spawned)
	}
	if manager.created.Directory != "repo" || !manager.created.Worktree {
		t.Fatalf("create request = %#v, want Directory=repo Worktree=true", manager.created)
	}
	var session storage.Session
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.SessionID != "" {
		t.Fatalf("session runtime ref = %#v, want stored acp session without runtime session id", session.RuntimeRef)
	}
}

func TestCreateACPSessionRequiresDirectoryForWorktree(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{spawnStore: store}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"runtime":"acp","agent":"codex","worktree":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "worktree requires") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.created.ACPAgent != "" || manager.spawned.ACPAgent != "" {
		t.Fatalf("manager was called: created=%#v spawned=%#v", manager.created, manager.spawned)
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
	if manager.spawned.ACPAgent != "" {
		t.Fatalf("create should defer eager spawn: %#v", manager.spawned)
	}
	if manager.created.Model != "sonnet" {
		t.Fatalf("create request = %#v, want Model=sonnet", manager.created)
	}
}

func TestSessionPinRoutesKeepProjectPath(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	session, err := store.CreateSession(storage.CreateSession{
		Slug: "pin-me",
		RuntimeRef: &storage.RuntimeRef{
			Type:        storage.RuntimeACP,
			Cwd:         project,
			ProjectPath: project,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()

	pinReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/pin", nil)
	pinRes := httptest.NewRecorder()
	handler.ServeHTTP(pinRes, pinReq)
	if pinRes.Code != http.StatusOK {
		t.Fatalf("pin status = %d, body = %s", pinRes.Code, pinRes.Body.String())
	}
	var pinned storage.Session
	if err := json.Unmarshal(pinRes.Body.Bytes(), &pinned); err != nil {
		t.Fatal(err)
	}
	if !pinned.Pinned || pinned.RuntimeRef == nil || pinned.RuntimeRef.ProjectPath != project {
		t.Fatalf("pinned response = %#v, want pinned with project %q intact", pinned, project)
	}

	unpinReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/unpin", nil)
	unpinRes := httptest.NewRecorder()
	handler.ServeHTTP(unpinRes, unpinReq)
	if unpinRes.Code != http.StatusOK {
		t.Fatalf("unpin status = %d, body = %s", unpinRes.Code, unpinRes.Body.String())
	}
	var unpinned storage.Session
	if err := json.Unmarshal(unpinRes.Body.Bytes(), &unpinned); err != nil {
		t.Fatal(err)
	}
	if unpinned.Pinned || unpinned.RuntimeRef == nil || unpinned.RuntimeRef.ProjectPath != project {
		t.Fatalf("unpinned response = %#v, want project %q intact", unpinned, project)
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
	alpha := filepath.Join(t.TempDir(), "alpha")
	if err := os.MkdirAll(alpha, 0o755); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()

	projectBody := func(path string) string {
		raw, err := json.Marshal(path)
		if err != nil {
			t.Fatal(err)
		}
		return `{"path":` + string(raw) + `}`
	}
	body := projectBody(dir)
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

	req = httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(projectBody(alpha)))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("second create status = %d, body = %s", res.Code, res.Body.String())
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
	if len(got.Projects) != 2 ||
		got.Projects[0].Name != "jaz" ||
		got.Projects[0].Path != dir ||
		!got.Projects[0].Git ||
		got.Projects[1].Name != "alpha" ||
		got.Projects[1].Path != alpha {
		t.Fatalf("projects = %#v, want creation order jaz then alpha", got.Projects)
	}

	rawAlpha, err := json.Marshal(alpha)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPut, "/v1/projects/order", strings.NewReader(`{"paths":[`+string(rawAlpha)+`]}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("reorder status = %d, body = %s", res.Code, res.Body.String())
	}
	got.Projects = nil
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Projects) != 2 || got.Projects[0].Path != alpha || got.Projects[1].Path != dir {
		t.Fatalf("reordered projects = %#v, want alpha then jaz", got.Projects)
	}

	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	rawOther, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPut, "/v1/projects/order", strings.NewReader(`{"paths":[`+string(rawOther)+`]}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unknown reorder status = %d, body = %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("malformed create status = %d, body = %s", res.Code, res.Body.String())
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

func TestListFilesystemDirsDefaultsToWorkspace(t *testing.T) {
	workspace := t.TempDir()
	req := httptest.NewRequest(http.MethodGet, "/v1/filesystem/dirs", nil)
	res := httptest.NewRecorder()
	(&Server{Workspace: workspace}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != workspace {
		t.Fatalf("path = %q, want workspace %q", got.Path, workspace)
	}
}

func TestListFilesystemDirsResolvesRelativePathInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/filesystem/dirs?path=repo", nil)
	res := httptest.NewRecorder()
	(&Server{Workspace: workspace}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Path string `json:"path"`
		Git  bool   `json:"git"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != repo || !got.Git {
		t.Fatalf("dir = %#v, want git repo %q", got, repo)
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
	req.Header.Set("X-Jaz-Client-Platform", "mobile")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	sendCtxErr := manager.sendCtxErr
	sendPlatform := manager.sendPlatform
	manager.mu.Unlock()
	if sendCtxErr != nil {
		t.Fatalf("acp send used cancelled request context: %v", sendCtxErr)
	}
	if sendPlatform != "mobile" {
		t.Fatalf("acp send platform = %q, want mobile", sendPlatform)
	}
}

func TestACPSideChatRoutesToManager(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-side-chat",
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
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateRunning}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/side-chat", strings.NewReader(`{"id":"side-1","message":"quick check"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	sideChat := manager.sideChat
	sideCtxErr := manager.sideCtxErr
	manager.mu.Unlock()
	if sideCtxErr != nil {
		t.Fatalf("side chat used cancelled request context: %v", sideCtxErr)
	}
	if sideChat.Session != session.ID || sideChat.ID != "side-1" || sideChat.Message != "quick check" {
		t.Fatalf("side chat request = %#v", sideChat)
	}
}

func TestACPSideChatRejectsNonCodexSession(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-wrapper-side-chat",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex-wrapper",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateRunning}}
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/side-chat", strings.NewReader(`{"id":"side-1","message":"quick check"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	sideChat := manager.sideChat
	manager.mu.Unlock()
	if sideChat.Session != "" {
		t.Fatalf("side chat manager was called: %#v", sideChat)
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
		Runtime: storage.RuntimeACP,
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
		Assistant:     "child answer",
		Thought:       "child thought",
		Plan:          []sessionevents.ACPPlanEntry{{Content: "Inspect current page", Status: "completed"}},
		ToolCalls:     []sessionevents.ACPToolCall{{ID: "tool-1", Title: "read file"}},
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
	if childState.ID != child.ID || childState.Slug != child.Slug || childState.ParentID != parent.ID {
		t.Fatalf("child state = %#v", childState)
	}
	if !childState.ParentVisible {
		t.Fatalf("parent_visible = false, want true")
	}
	if childState.Assistant != "" || childState.Thought != "" || len(childState.Plan) != 0 || len(childState.ToolCalls) != 0 || len(childState.Permissions) != 0 {
		t.Fatalf("child transcript leaked into parent response: %#v", childState)
	}
}

func TestSessionMessagesTreatsStoredACPStateAsInactive(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-stale",
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
		Plan:       []sessionevents.ACPPlanEntry{{Content: "Inspect current page", Status: "completed"}},
		Permissions: []sessionevents.ACPPermission{{
			ID:     "perm-1",
			Status: "pending",
			Questions: []sessionevents.ACPQuestion{{
				ID:       "audience",
				Question: "Who is the page for?",
			}},
		}},
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
		Session        storage.Session               `json:"session"`
		ACPState       string                        `json:"acp_state"`
		ACPPlan        []sessionevents.ACPPlanEntry  `json:"acp_plan"`
		ACPPermissions []sessionevents.ACPPermission `json:"acp_permissions"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Session.Status != storage.StatusIdle || got.ACPState != acp.StateIdle {
		t.Fatalf("status = %q, acp_state = %q", got.Session.Status, got.ACPState)
	}
	if len(got.ACPPlan) != 1 || got.ACPPlan[0].Content != "Inspect current page" {
		t.Fatalf("plan = %#v", got.ACPPlan)
	}
	if len(got.ACPPermissions) != 0 {
		t.Fatalf("permissions = %#v", got.ACPPermissions)
	}
}

func TestSessionMessagesIncludesPersistedSessionEvents(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:            "codex",
		Runtime:         storage.RuntimeACP,
		ModelProvider:   "codex",
		Model:           "gpt-5.5",
		ReasoningEffort: "xhigh",
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
		Events  []sessionevents.Event   `json:"events"`
		ACPMeta map[string]acpMetaEntry `json:"acp_meta"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || got.Events[0].Type != "acp_message" || got.Events[0].Content != "I inspected the workspace." {
		t.Fatalf("events = %#v", got.Events)
	}
	meta := got.ACPMeta[session.ID]
	if meta.ModelProvider != "codex" || meta.Model != "gpt-5.5" || meta.ReasoningEffort != "xhigh" {
		t.Fatalf("acp meta = %#v", got.ACPMeta)
	}
}

func TestSessionMessagesIncludesDirectACPChildMetadataOnly(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{Slug: "parent", Runtime: storage.RuntimeACP})
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
		Thought:    "private chain",
		Plan:       []sessionevents.ACPPlanEntry{{Content: "Inspect current page", Status: "completed"}},
		ToolCalls:  []sessionevents.ACPToolCall{{ID: "tool-1", Title: "read file"}},
		Permissions: []sessionevents.ACPPermission{{
			ID:     "perm-1",
			Status: "pending",
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
	state := got.ACPChildren[0]
	if state.ID != child.ID || state.Slug != child.Slug || state.ParentID != parent.ID {
		t.Fatalf("child state = %#v", state)
	}
	if state.ParentVisible {
		t.Fatalf("parent_visible = true, want false")
	}
	if state.Assistant != "" || state.Thought != "" || len(state.Plan) != 0 || len(state.ToolCalls) != 0 || len(state.Permissions) != 0 {
		t.Fatalf("child transcript leaked into parent response: %#v", state)
	}
}
