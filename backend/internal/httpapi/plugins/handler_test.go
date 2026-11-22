package plugins

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testgit"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/internal/visualize"
)

func makeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=" + os.DevNull}, args...)...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "--quiet")
	for rel, body := range files {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runGit("add", "--all")
	testgit.Snapshot(t, repo)
	return repo
}

func TestPluginsAreInstalledListedToggledAndRemovedOverHTTP(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "status.json"), []byte(`{"title":"Tender A"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	layout := runtimefiles.New(t.TempDir())
	db, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := plugins.New(layout)
	service := plugins.NewService(store, db, nil, nil, nil, nil)
	runtime := mcpruntime.NewManager(service, nil, tools.NewRegistry(), log.New(io.Discard))
	defer runtime.Close()
	handler := NewHandler(service, sessionStore{{ID: "thread", Title: "Tender work", Folder: folder, Reference: "tenders/status", File: "status.json", Seq: 1}}, runtime, "")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/plugins", handler.List)
	mux.HandleFunc("GET /v1/plugins/{plugin}", handler.Get)
	mux.HandleFunc("PUT /v1/plugins/{plugin}/enabled", handler.SetEnabled)
	mux.HandleFunc("GET /v1/plugins/{plugin}/page", handler.Page)
	mux.HandleFunc("POST /v1/plugins/install", handler.Install)
	mux.HandleFunc("DELETE /v1/plugins/{plugin}", handler.Remove)
	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(method, path, bytes.NewReader(body)))
		return recorder
	}

	repo := makeRepo(t, map[string]string{
		"tenders/plugin.json":       `{"name":"tenders","displayName":"Tender work","logo":"./assets/icon.svg","page":{"view":"status"}}`,
		"tenders/assets/icon.svg":   `<svg xmlns="http://www.w3.org/2000/svg"><path d="M1 1h12v12H1z"/></svg>`,
		"tenders/views/status.html": "<p>status</p>",
		"plain/plugin.json":         `{"name":"plain"}`,
	})
	if got := call(http.MethodPost, "/v1/plugins/install", []byte("not json")); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid json = %d, want 400", got.Code)
	}
	payload, _ := json.Marshal(plugins.RepositoryInput{Repository: repo})
	if got := call(http.MethodPost, "/v1/plugins/install", payload); got.Code != http.StatusOK {
		t.Fatalf("install = %d: %s", got.Code, got.Body)
	}
	var listed plugins.CatalogResponse
	if err := json.Unmarshal(call(http.MethodGet, "/v1/plugins", nil).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Plugins) != 2 || listed.Plugins[0].Page != nil || listed.Plugins[1].Page == nil || listed.Plugins[1].DisplayName != "Tender work" || listed.Plugins[1].Page.View != "status" {
		t.Fatalf("listed = %#v, want the tenders plugin with its status view", listed.Plugins)
	}
	if listed.Plugins[1].IconDataURL != "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M1 1h12v12H1z"/></svg>`)) {
		t.Fatalf("icon = %q, want the installed SVG", listed.Plugins[1].IconDataURL)
	}
	// Toggle enabled off and back on.
	if got := call(http.MethodPut, "/v1/plugins/tenders/enabled", []byte(`{"enabled":false}`)); got.Code != http.StatusOK {
		t.Fatalf("disable = %d: %s", got.Code, got.Body)
	}
	var disabled plugins.Plugin
	if err := json.Unmarshal(call(http.MethodGet, "/v1/plugins/tenders", nil).Body.Bytes(), &disabled); err != nil || disabled.Enabled {
		t.Fatalf("expected disabled plugin: %+v %v", disabled, err)
	}
	if got := call(http.MethodPut, "/v1/plugins/tenders/enabled", []byte(`{"enabled":true}`)); got.Code != http.StatusOK {
		t.Fatalf("enable = %d: %s", got.Code, got.Body)
	}

	response := call(http.MethodGet, "/v1/plugins/tenders/page", nil)
	var page plugins.PageContent
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.View != "<p>status</p>" || len(page.Items) != 1 || page.Items[0].Folder != folder || page.Items[0].Data.(map[string]any)["title"] != "Tender A" || page.Items[0].Threads[0].ID != "thread" {
		t.Fatalf("page = %d: %s", response.Code, response.Body)
	}
	for _, path := range []string{"/v1/plugins/plain/page", "/v1/plugins/missing/page"} {
		if response := call(http.MethodGet, path, nil); response.Code != http.StatusNotFound {
			t.Fatalf("%s = %d: %s", path, response.Code, response.Body)
		}
	}
	if got := call(http.MethodDelete, "/v1/plugins/tenders", nil); got.Code != http.StatusNoContent {
		t.Fatalf("remove = %d, want 204", got.Code)
	}
	if got := call(http.MethodDelete, "/v1/plugins/tenders", nil); got.Code != http.StatusNotFound {
		t.Fatalf("removing a plugin that is not installed = %d, want 404", got.Code)
	}
}

type sessionStore []storage.SessionPluginView

func (s sessionStore) ListSessionPluginViews() ([]storage.SessionPluginView, error) {
	return s, nil
}

func TestOpenViewKeepsTheSavedCallerPluginFileAndThread(t *testing.T) {
	root := t.TempDir()
	layout := runtimefiles.New(root)
	packages := plugins.New(layout)
	for _, plugin := range []string{"example", "other"} {
		dir := filepath.Join(layout.Plugins, plugin, "views")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "quiz.html"), []byte("<p>"+plugin+" quiz</p>"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Close()
	thread, err := sessions.CreateSession(storage.CreateSession{Slug: "view-owner"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := sessions.CreateSession(storage.CreateSession{Slug: "other-thread"})
	if err != nil {
		t.Fatal(err)
	}
	bus := sessionevents.New()
	presenter := &visualize.Service{Store: sessions, Sessions: sessions, Events: bus}
	caller := sessionevents.PluginView{Reference: "example/status", File: "status.json", Title: "Status", WidgetCode: "<p>Status</p>"}
	if _, err := presenter.Present(thread.ID, &sessionevents.PresentationEvent{Kind: sessionevents.PresentationPluginView, View: &caller}); err != nil {
		t.Fatal(err)
	}
	handler := NewViewHandler(plugins.NewViewService(sessions, presenter, packages, layout))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/sessions/{session}/plugin-views", handler.Open)
	call := func(session, reference, file, name string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"reference": reference, "file": file, "name": name, "title": "Quiz"})
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session+"/plugin-views", bytes.NewReader(body)))
		return response
	}
	for _, name := range []string{"other/quiz", "../quiz", `other\quiz`, "quiz.html", "/quiz"} {
		if response := call(thread.ID, caller.Reference, caller.File, name); response.Code != http.StatusBadRequest {
			t.Fatalf("unsafe view %q = %d: %s", name, response.Code, response.Body)
		}
	}
	for _, response := range []*httptest.ResponseRecorder{
		call(other.ID, caller.Reference, caller.File, "quiz"),
		call("missing-thread", caller.Reference, caller.File, "quiz"),
		call(thread.ID, "other/status", caller.File, "quiz"),
		call(thread.ID, caller.Reference, "other.json", "quiz"),
	} {
		if response.Code != http.StatusNotFound {
			t.Fatalf("changed caller = %d: %s", response.Code, response.Body)
		}
	}
	events := bus.Subscribe(t.Context(), thread.ID)
	response := call(thread.ID, caller.Reference, caller.File, "quiz")
	var target sessionevents.PluginView
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &target) != nil || target.Reference != "example/quiz" || target.File != caller.File || target.WidgetCode != "<p>example quiz</p>" {
		t.Fatalf("opened view = %d: %s", response.Code, response.Body)
	}
	select {
	case event := <-events:
		if event.SessionID != thread.ID || event.Artifact.View.Reference != target.Reference {
			t.Fatalf("presentation = %+v", event)
		}
	default:
		t.Fatal("opened view was not published")
	}
	saved, err := sessions.LoadSessionOverview(t.Context(), thread.ID)
	if err != nil || len(sessionevents.LatestPluginViews(saved.PresentationEvents)) != 2 {
		t.Fatalf("opened view not discoverable after return: %+v, %v", saved, err)
	}
}

func TestViewStateSupportsETagAndIfMatchConflict(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	sessions, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Close()
	thread, err := sessions.CreateSession(storage.CreateSession{Slug: "answering"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := sessions.CreateSession(storage.CreateSession{Slug: "elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	presenter := &visualize.Service{Store: sessions, Sessions: sessions, Events: sessionevents.New()}
	shown := sessionevents.PluginView{Reference: "example/status", File: "status.json", Title: "Status", WidgetCode: "<p>Status</p>"}
	if _, err := presenter.Present(thread.ID, &sessionevents.PresentationEvent{Kind: sessionevents.PresentationPluginView, View: &shown}); err != nil {
		t.Fatal(err)
	}
	handler := NewViewHandler(plugins.NewViewService(sessions, presenter, plugins.New(layout).WithStateStore(sessions), layout))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/sessions/{session}/plugin-views/state", handler.State)
	mux.HandleFunc("PUT /v1/sessions/{session}/plugin-views/state", handler.SaveState)
	call := func(method, session, reference, file, body, ifMatch string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		target := "/v1/sessions/" + session + "/plugin-views/state?reference=" + url.QueryEscape(reference) + "&file=" + url.QueryEscape(file)
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if ifMatch != "" {
			req.Header.Set("If-Match", ifMatch)
		}
		mux.ServeHTTP(response, req)
		return response
	}
	state := func() (string, string) {
		response := call(http.MethodGet, thread.ID, shown.Reference, shown.File, "", "")
		if response.Code != http.StatusOK {
			t.Fatalf("state = %d: %s", response.Code, response.Body)
		}
		return strings.TrimSpace(response.Body.String()), response.Header().Get("ETag")
	}
	if got, _ := state(); got != "null" {
		t.Fatalf("state before any save = %s", got)
	}
	resp := call(http.MethodPut, thread.ID, shown.Reference, shown.File, `{"answers":[[3,"Accept"]]}`, "")
	if resp.Code != http.StatusNoContent || resp.Header().Get("ETag") != `"1"` {
		t.Fatalf("first save = %d etag=%q: %s", resp.Code, resp.Header().Get("ETag"), resp.Body)
	}
	// Stale If-Match returns 409 Conflict.
	if resp := call(http.MethodPut, thread.ID, shown.Reference, shown.File, `{"answers":[]}`, `"0"`); resp.Code != http.StatusConflict {
		t.Fatalf("stale If-Match = %d, want 409", resp.Code)
	}
	// Matching If-Match succeeds and bumps ETag.
	resp = call(http.MethodPut, thread.ID, shown.Reference, shown.File, `{"answers":[[3,"Accept"],[7,"Deviate"]]}`, `"1"`)
	if resp.Code != http.StatusNoContent || resp.Header().Get("ETag") != `"2"` {
		t.Fatalf("second save = %d etag=%q: %s", resp.Code, resp.Header().Get("ETag"), resp.Body)
	}
	if got, etag := state(); got != `{"answers":[[3,"Accept"],[7,"Deviate"]]}` || etag != `"2"` {
		t.Fatalf("state = %s etag=%q", got, etag)
	}
	for name, response := range map[string]*httptest.ResponseRecorder{
		"another thread reads":  call(http.MethodGet, other.ID, shown.Reference, shown.File, "", ""),
		"another thread saves":  call(http.MethodPut, other.ID, shown.Reference, shown.File, `1`, ""),
		"a view never shown":    call(http.MethodPut, thread.ID, "example/quiz", shown.File, `1`, ""),
		"a file never followed": call(http.MethodPut, thread.ID, shown.Reference, "other.json", `1`, ""),
		"a thread that is gone": call(http.MethodGet, "missing-thread", shown.Reference, shown.File, "", ""),
	} {
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s = %d: %s", name, response.Code, response.Body)
		}
	}
	for name, response := range map[string]*httptest.ResponseRecorder{
		"a path as the view":    call(http.MethodPut, thread.ID, "../../status", shown.File, `1`, ""),
		"a path as the file":    call(http.MethodPut, thread.ID, shown.Reference, "../status.json", `1`, ""),
		"text that is not JSON": call(http.MethodPut, thread.ID, shown.Reference, shown.File, `{"answers":`, ""),
	} {
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d: %s", name, response.Code, response.Body)
		}
	}
	if response := call(http.MethodPut, thread.ID, shown.Reference, shown.File, `"`+strings.Repeat("a", maxViewStateBytes)+`"`, ""); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized state = %d: %s", response.Code, response.Body)
	}
}
