package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestMCPDiscoveryPreservesVariablesAndIsolatesBadEntries(t *testing.T) {
	layout := runtimefiles.New(filepath.Join(t.TempDir(), "literal-${PLUGIN_DATA}"))
	store := New(layout)
	installed, err := installFiles(t, store, map[string]string{
		"fixture/plugin.json":       `{"name":"fixture","page":{"view":"status"}}`,
		"fixture/views/status.html": "<p>status</p>",
		"fixture/mcp.json": `{"mcpServers":{
"local":{"type":"stdio","command":"printf","cwd":"${PLUGIN_DATA}","args":["${PLUGIN_ROOT}","${PLUGIN_DATA}","${UNKNOWN}","${UNKNOWN:-literal}"],"env":{"VALUE":"${PLUGIN_ROOT}"}},
"default":{"type":"stdio","command":"printf"},
"remote":{"type":"streamable-http","url":"https://example.test/${PLUGIN_ROOT}","headers":{"X-Value":"${PLUGIN_ROOT}"}},
"events":{"type":"sse","url":"http://127.0.0.1:12345/sse"},
"reserved":{"type":"stdio","command":"printf","env":{"PLUGIN_ROOT":"bad"}},
"unknown":{"type":"stdio","command":"printf","url":"https://example.test"},
"escape":{"type":"stdio","command":"../outside"},
"cwd-escape":{"type":"stdio","command":"printf","cwd":"${PLUGIN_ROOT}/../outside"},
"http":{"type":"streamable-http","url":"http://example.test/mcp"},
"headers":{"type":"sse","url":"https://example.test","headers":{"X-Value":"one","x-value":"two"}}
}}`,
	})
	if err != nil || len(installed) != 1 || len(installed[0].MCPServers) != 4 || len(installed[0].Warnings) != 6 {
		t.Fatalf("inventory = %+v, %v", installed, err)
	}
	servers, err := store.ListMCPServers()
	if err != nil || len(servers) != 4 {
		t.Fatalf("servers = %+v, %v", servers, err)
	}
	root := filepath.Join(layout.Plugins, "fixture")
	data := filepath.Join(layout.Root, "plugin-data", "fixture")
	for _, server := range servers {
		if server.Source != mcpconfig.SourcePlugin || !server.Enabled || !strings.HasPrefix(server.ID, "plugin:fixture:") {
			t.Fatalf("server ownership = %+v", server)
		}
		switch server.Name {
		case "fixture:local":
			if server.Cwd != data || server.Args[0] != root || server.Args[1] != data || server.Args[2] != "${UNKNOWN}" || server.Args[3] != "literal" || server.Env["VALUE"] != root || server.Env["PLUGIN_ROOT"] != root || server.Env["PLUGIN_DATA"] != data {
				t.Fatalf("portable expansion = %+v", server)
			}
		case "fixture:default":
			if server.Cwd != root {
				t.Fatalf("default cwd = %q", server.Cwd)
			}
		case "fixture:remote":
			if server.Headers[0].Value != root {
				t.Fatalf("remote header expansion = %+v", server.Headers)
			}
		case "fixture:events":
			if server.Transport != mcpconfig.TransportSSE {
				t.Fatal("SSE transport changed")
			}
		}
	}
	if err := os.WriteFile(filepath.Join(root, "mcp.json"), []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if page, err := store.Page("fixture", pageSessions{}); err != nil || page.View != "<p>status</p>" {
		t.Fatalf("malformed MCP blocked the plugin page: %+v %v", page, err)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatalf("inventory created plugin data before process startup: %v", err)
	}
	if err := store.Remove("fixture"); err != nil {
		t.Fatal(err)
	}
	if servers, err := store.ListMCPServers(); err != nil || len(servers) != 0 {
		t.Fatalf("removed servers = %+v, %v", servers, err)
	}
}

func TestNativeMCPDefaultsPathsAndAntigravityMCPConfigVariables(t *testing.T) {
	t.Setenv("JAZ_PLUGIN_TEST", "configured")
	t.Setenv("MISSING_PLUGIN_VAR", "")
	for _, layoutDir := range []string{".claude-plugin", ".codex-plugin"} {
		store := New(runtimefiles.New(t.TempDir()))
		installed, err := installFiles(t, store, map[string]string{
			"fixture/" + layoutDir + "/plugin.json": `{"name":"fixture","mcpServers":["./extra.json",{"shared":{"type":"http","url":"https://example.test/${JAZ_PLUGIN_TEST}","headers":{"X-Default":"${MISSING_PLUGIN_VAR:-fallback}"}}}]}`,
			"fixture/.mcp.json":                     `{"mcpServers":{"local":{"command":"${CLAUDE_PLUGIN_ROOT}/server","args":["${JAZ_PLUGIN_TEST}"],"env":{"VALUE":"${MISSING_PLUGIN_VAR:-fallback}"}},"shared":{"type":"http","url":"https://default.test"}}}`,
			"fixture/extra.json":                    `{"mcpServers":{"extra":{"type":"sse","url":"https://example.test/events"}}}`,
		})
		if err != nil || len(installed) != 1 || len(installed[0].Warnings) != 0 {
			t.Fatalf("native inventory = %+v, %v", installed, err)
		}
		servers, err := store.ListMCPServers()
		if err != nil || len(servers) != 3 {
			t.Fatalf("native servers = %+v, %v", servers, err)
		}
		for _, server := range servers {
			if server.Name == "fixture:shared" && (server.URL != "https://example.test/configured" || server.Headers[0].Value != "fallback") {
				t.Fatalf("native overrides = %+v", server)
			}
			if server.Name == "fixture:local" && (!strings.HasSuffix(server.Command, "/fixture/server") || server.Args[0] != "configured" || server.Env["VALUE"] != "fallback") {
				t.Fatalf("native expansion = %+v", server)
			}
		}
		encoded, err := json.Marshal(installed)
		if err != nil || strings.Contains(string(encoded), "configured") || strings.Contains(string(encoded), "fallback") {
			t.Fatal("inventory exposed MCP configuration values")
		}
	}

	// Antigravity mcp_config.json with serverUrl + variables[] + saved config + disable toggle.
	layout := runtimefiles.New(t.TempDir())
	db, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(layout)
	svc := NewService(store, db, nil, nil, nil, nil)
	if _, err := installFiles(t, store, map[string]string{
		"agy-demo/plugin.json": `{"name":"agy-demo","displayName":"AGY Demo"}`,
		"agy-demo/mcp_config.json": `{
			"mcpServers": {
				"api": {
					"serverUrl": "https://api.example.com/mcp",
					"headers": {"Authorization": "Bearer ${API_TOKEN}"},
					"variables": [{"name":"API_TOKEN","label":"API Token","secret":true,"required":true}]
				}
			}
		}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveConfig(t.Context(), "agy-demo", map[string]string{"API_TOKEN": "tok-123"}); err != nil {
		t.Fatal(err)
	}
	servers, err := svc.ListMCPServers()
	if err != nil || len(servers) != 1 || servers[0].URL != "https://api.example.com/mcp" || len(servers[0].Headers) != 1 || servers[0].Headers[0].Value != "Bearer tok-123" {
		t.Fatalf("agy mcp servers = %+v, %v", servers, err)
	}
	got, err := svc.Get(t.Context(), "agy-demo", false)
	if err != nil || len(got.ConfigVariables) != 1 || !got.ConfigSet["API_TOKEN"] {
		t.Fatalf("config variables or configured flag missing: %+v, %v", got, err)
	}
	if _, err := svc.SetEnabled(t.Context(), "agy-demo", false); err != nil {
		t.Fatal(err)
	}
	servers, err = svc.ListMCPServers()
	if err != nil || len(servers) != 0 {
		t.Fatalf("disabled plugin still exposed MCP servers: %+v, %v", servers, err)
	}
	claudeDirs, err := svc.ClaudePlugins()
	if err != nil || len(claudeDirs) != 0 {
		t.Fatalf("disabled plugin still exposed Claude plugin dir: %+v, %v", claudeDirs, err)
	}
}
