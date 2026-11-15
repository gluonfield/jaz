package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestPluginsAreInstalledListedAndRemovedOverHTTP(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "status.json"), []byte(`{"title":"Tender A"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(plugins.New(runtimefiles.New(t.TempDir())), sessionStore{{ID: "thread", Title: "Tender work", RuntimeRef: &storage.RuntimeRef{Cwd: folder}}})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/plugins", handler.List)
	mux.HandleFunc("GET /v1/plugins/{plugin}/page", handler.Page)
	mux.HandleFunc("POST /v1/plugins", handler.Install)
	mux.HandleFunc("DELETE /v1/plugins/{plugin}", handler.Remove)
	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(method, path, bytes.NewReader(body)))
		return recorder
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range map[string]string{
		"tenders/plugin.json":       `{"name":"tenders","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","extensions":{"chat.jaz":{"displayName":"Tender work","icon":"./assets/icon.svg","page":{"view":"status","file":"status.json"}}}}`,
		"tenders/assets/icon.svg":   `<svg xmlns="http://www.w3.org/2000/svg"><path d="M1 1h12v12H1z"/></svg>`,
		"tenders/views/status.html": "<p>status</p>",
		"plain/plugin.json":         `{"name":"plain","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"}`,
	} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	if got := call(http.MethodPost, "/v1/plugins", []byte("not a zip")); got.Code != http.StatusBadRequest {
		t.Fatalf("installing a file that is not a zip = %d, want 400", got.Code)
	}
	if got := call(http.MethodPost, "/v1/plugins", archive.Bytes()); got.Code != http.StatusOK {
		t.Fatalf("install = %d: %s", got.Code, got.Body)
	}
	var listed listResponse
	if err := json.Unmarshal(call(http.MethodGet, "/v1/plugins", nil).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Plugins) != 2 || listed.Plugins[0].Page != nil || listed.Plugins[1].Page == nil || listed.Plugins[1].DisplayName != "Tender work" || listed.Plugins[1].Page.File != "status.json" {
		t.Fatalf("listed = %#v, want the tenders plugin with its status view", listed.Plugins)
	}
	if listed.Plugins[1].IconDataURL != "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M1 1h12v12H1z"/></svg>`)) {
		t.Fatalf("icon = %q, want the installed SVG", listed.Plugins[1].IconDataURL)
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

type sessionStore []storage.Session

func (s sessionStore) ListSessions(filter storage.SessionFilter) ([]storage.Session, error) {
	var matches []storage.Session
	for _, session := range s {
		if storage.SessionMatchesFilter(session, filter) {
			matches = append(matches, session)
		}
	}
	return matches, nil
}
