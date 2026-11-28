package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
)

func TestCreateACPSessionUsesTitleForInitialSlug(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{spawnStore: store}

	body := `{"runtime":"acp","agent":"claude","title":"Repair thread title slugs","directory":"repo","worktree":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.created.Title != "Repair thread title slugs" {
		t.Fatalf("create request = %#v, want title forwarded", manager.created)
	}
	var session storage.Session
	if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Slug != "repair-thread-title-slugs" {
		t.Fatalf("slug = %q, want task-derived slug", session.Slug)
	}
	if session.ModelProvider != acp.AgentClaude {
		t.Fatalf("model provider = %q, want %q", session.ModelProvider, acp.AgentClaude)
	}
}

func TestProjectlessChatsKeepSeparateFilesAfterRestart(t *testing.T) {
	root := t.TempDir()
	t.Chdir(filepath.Dir(root))
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := acp.Config{
		Root:      filepath.Base(root),
		Workspace: filepath.Join(root, "workspaces", "default"),
		Agents:    map[string]acp.AgentConfig{"fake": {Command: "fake"}},
	}
	manager := acp.NewManager(store, cfg, nil)
	t.Cleanup(manager.Close)
	handler := (&Server{Store: store, ACP: manager, Root: root, Workspace: cfg.Workspace}).Handler()
	var sessions []storage.Session
	contents := []string{"first chat", "second chat"}
	for i, body := range []string{`{"agent":"fake"}`, `{"agent":"fake","directory":"  "}`} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(body)))
		if res.Code != http.StatusOK {
			t.Fatalf("create status = %d, body = %s", res.Code, res.Body.String())
		}
		var session storage.Session
		if err := json.Unmarshal(res.Body.Bytes(), &session); err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(root, "chats", session.ID)
		if session.RuntimeRef == nil || session.RuntimeRef.Cwd != want || session.RuntimeRef.ProjectPath != "" {
			t.Fatalf("runtime ref = %#v, want cwd %q and no project", session.RuntimeRef, want)
		}
		if err := os.WriteFile(filepath.Join(want, "result.txt"), []byte(contents[i]), 0o600); err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, session)
	}
	if err := store.UpdateSessionTitle(sessions[0].ID, "Renamed chat"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetArchived(sessions[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetArchived(sessions[0].ID, false); err != nil {
		t.Fatal(err)
	}
	manager.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler = (&Server{Store: store, Root: root, Workspace: cfg.Workspace}).Handler()
	for i, session := range sessions {
		loaded, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.RuntimeRef.Cwd != session.RuntimeRef.Cwd || loaded.RuntimeRef.ProjectPath != "" {
			t.Fatalf("directory changed after restart: %#v", loaded.RuntimeRef)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/file?path=result.txt&raw=1", nil))
		if res.Code != http.StatusOK || res.Body.String() != contents[i] {
			t.Fatalf("file for chat %s = %d %q, want %q", session.ID, res.Code, res.Body.String(), contents[i])
		}
	}
}

func TestNewThreadOpensWhileAgentStarts(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	silentAgent := acp.AgentConfig{Command: "/bin/sh", Args: []string{"-c", "cat >/dev/null"}, Model: "opus"}
	manager := acp.NewManager(store, acp.Config{
		Root:      root,
		Workspace: workspace,
		Agents:    map[string]acp.AgentConfig{acp.AgentClaude: silentAgent},
	}, nil)
	t.Cleanup(manager.Close)
	handler := (&Server{Store: store, ACP: manager, Routes: Routes{{
		Pattern: "GET /v1/sessions/{session}/messages",
		Handler: sessionsapi.NewMessagesHandler(transcript.NewService(store, manager)),
	}}}).Handler()
	serve := func(method, path, body string) []byte {
		t.Helper()
		res := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			handler.ServeHTTP(res, httptest.NewRequest(method, path, strings.NewReader(body)))
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s %s waited for the agent to start", method, path)
		}
		if res.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d, body = %s", method, path, res.Code, res.Body.String())
		}
		return res.Body.Bytes()
	}

	var session storage.Session
	if err := json.Unmarshal(serve(http.MethodPost, "/v1/sessions", `{"agent":"claude"}`), &session); err != nil {
		t.Fatal(err)
	}
	serve(http.MethodPost, "/v1/sessions/"+session.ID+"/queue", `{"op":"append","message":{"text":"hello"}}`)
	var view struct {
		Session  storage.Session `json:"session"`
		ACPState string          `json:"acp_state"`
	}
	if err := json.Unmarshal(serve(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", ""), &view); err != nil {
		t.Fatal(err)
	}
	if view.Session.Status != storage.StatusRunning || view.ACPState != acp.StateStarting {
		t.Fatalf("thread status = %q, acp state = %q; want running while the agent starts", view.Session.Status, view.ACPState)
	}
}

func TestCreateACPSessionFixesTheArtifactSurface(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{spawnStore: store}
	create := func(surface string) *httptest.ResponseRecorder {
		body := `{"runtime":"acp","agent":"claude","artifact_surface":"` + surface + `"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(body))
		res := httptest.NewRecorder()
		(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)
		return res
	}

	if res := create("canvas"); res.Code != http.StatusOK || manager.created.ArtifactSurface != "canvas" {
		t.Fatalf("status = %d, surface = %q, body = %s", res.Code, manager.created.ArtifactSurface, res.Body.String())
	}
	if res := create("widget"); res.Code != http.StatusBadRequest {
		t.Fatalf("widget surface status = %d, want 400: loop widgets are not a thread mode", res.Code)
	}
}

func TestNativeCloudProfileIsSavedWithItsWorkspace(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := acp.NewManager(store, acp.Config{Root: root, Agents: acp.BuiltinAgents()}, nil)
	defer manager.Close()
	cloud := &switchableCloudBridge{active: "workspace-a"}
	handler := (&Server{Root: root, Store: store, ACP: manager, Cloud: cloud}).Handler()
	create := func(workspaceID, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(body))
		req.Header.Set("X-Jaz-Cloud-Workspace", workspaceID)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	const native = `{"agent":"codex","cloud_credential_profile_id":"acp:codex"}`
	rec := create("workspace-a", native)
	if rec.Code != http.StatusOK {
		t.Fatalf("native create returned %d: %s", rec.Code, rec.Body)
	}
	var created storage.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	saved, err := store.LoadSession(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Backend == "cloud" || saved.WorkspaceID != "workspace-a" || saved.RuntimeRef == nil || saved.RuntimeRef.CloudCredentialProfileID != "acp:codex" {
		t.Fatalf("credential selection did not survive reload: %s", rec.Body)
	}
	cloud.active = "workspace-b"
	if rec := create("workspace-a", native); rec.Code != http.StatusConflict {
		t.Fatalf("stale create returned %d: %s", rec.Code, rec.Body)
	}
	const mixed = `{"agent":"codex","backend":"cloud","cloud_credential_profile_id":"acp:codex"}`
	if rec := create("workspace-b", mixed); rec.Code != http.StatusBadRequest {
		t.Fatalf("mixed launch returned %d: %s", rec.Code, rec.Body)
	}
}
