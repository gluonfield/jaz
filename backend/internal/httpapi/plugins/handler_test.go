package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
)

func TestPluginsAreInstalledListedAndRemovedOverHTTP(t *testing.T) {
	handler := NewHandler(plugins.New(runtimefiles.New(t.TempDir())))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/plugins", handler.List)
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
		"tenders/plugin.json":       `{"name":"tenders","title":"Tenders"}`,
		"tenders/views/status.html": "<p>status</p>",
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
	if len(listed.Plugins) != 1 || listed.Plugins[0].Title != "Tenders" || len(listed.Plugins[0].Views) != 1 || listed.Plugins[0].Views[0] != "status" {
		t.Fatalf("listed = %#v, want the tenders plugin with its status view", listed.Plugins)
	}
	if got := call(http.MethodDelete, "/v1/plugins/tenders", nil); got.Code != http.StatusNoContent {
		t.Fatalf("remove = %d, want 204", got.Code)
	}
	if got := call(http.MethodDelete, "/v1/plugins/tenders", nil); got.Code != http.StatusNotFound {
		t.Fatalf("removing a plugin that is not installed = %d, want 404", got.Code)
	}
}
