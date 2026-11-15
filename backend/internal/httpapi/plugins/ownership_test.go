package plugins_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/server"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestPluginMCPServersAreListedButCannotBeMutatedAsStoredServers(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	root := filepath.Join(layout.Plugins, "fixture")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"fixture"}`,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"remote":{"type":"sse","url":"https://example.invalid/mcp","headers":{"X-Token":"sensitive"}}}}`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	installed := plugins.New(layout)
	configs, err := installed.ListMCPServers()
	if err != nil || len(configs) != 1 {
		t.Fatalf("configs = %+v %v", configs, err)
	}
	manager := mcpruntime.NewManager(installed, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	handler := (&server.Server{Store: store, MCP: manager}).Handler()
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/v1/mcp/servers", nil))
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"source":"plugin"`) || strings.Contains(listed.Body.String(), "sensitive") {
		t.Fatalf("derived list = %d %s", listed.Code, listed.Body)
	}
	path := "/v1/mcp/servers/" + configs[0].ID
	for _, test := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPut, path, `{}`, http.StatusNotFound},
		{http.MethodDelete, path, "", http.StatusNotFound},
		{http.MethodPost, path + "/disable", "", http.StatusNotFound},
		{http.MethodPost, path + "/test", `{"name":"copy"}`, http.StatusBadRequest},
		{http.MethodPost, path + "/authorize", "", http.StatusOK},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if recorder.Code != test.status {
			t.Fatalf("%s %s = %d %s", test.method, test.path, recorder.Code, recorder.Body)
		}
		if strings.HasSuffix(test.path, "/authorize") && !strings.Contains(recorder.Body.String(), "Legacy SSE OAuth sign-in is not supported") {
			t.Fatalf("derived OAuth limitation was not explicit: %s", recorder.Body)
		}
	}
	if stored, err := store.ListMCPServers(); err != nil || len(stored) != 0 {
		t.Fatalf("derived config copied to storage = %+v %v", stored, err)
	}
}
