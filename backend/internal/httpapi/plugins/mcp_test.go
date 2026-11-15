package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestPluginMCPProcess(t *testing.T) {
	if os.Getenv("JAZ_PLUGIN_MCP_PROCESS") != "1" {
		return
	}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "plugin-fixture"}, nil)
	server.AddTool(&mcpsdk.Tool{Name: "inspect", InputSchema: map[string]any{"type": "object"}, Meta: mcpsdk.Meta{"ui": map[string]any{"resourceUri": "ui://fixture/app"}, "openai/ui": map[string]any{"entrypoints": []any{map[string]any{"type": "global"}}}}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		cwd, _ := os.Getwd()
		data, _ := json.Marshal(map[string]string{"pid": strconv.Itoa(os.Getpid()), "cwd": cwd, "root": os.Getenv("PLUGIN_ROOT"), "data": os.Getenv("PLUGIN_DATA"), "value": os.Getenv("VALUE")})
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}}}, nil
	})
	server.AddResource(&mcpsdk.Resource{URI: "ui://fixture/app", Name: "Plugin App", MIMEType: mcpruntime.AppMIMEType}, func(context.Context, *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{URI: "ui://fixture/app", MIMEType: mcpruntime.AppMIMEType, Text: "<!doctype html><h1>Plugin MCP App</h1>"}}}, nil
	})
	_ = server.Run(context.Background(), &mcpsdk.StdioTransport{})
	_ = os.WriteFile(filepath.Join(os.Getenv("PLUGIN_DATA"), "stopped"), nil, 0600)
	os.Exit(0)
}

func TestPluginInstallRefreshesProxyToolsAndMCPApps(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(binary, filepath.Join(bin, "plugin-fixture")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	layout := runtimefiles.New(filepath.Join(t.TempDir(), "literal-${PLUGIN_DATA}"))
	store := plugins.New(layout)
	manager := mcpruntime.NewManager(store, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	handler := NewHandler(sessionStore{}, manager, store)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/plugins", handler.Install)
	mux.HandleFunc("DELETE /v1/plugins/{plugin}", handler.Remove)
	install := func(value string) {
		t.Helper()
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		for name, content := range map[string]string{
			"fixture/plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"fixture"}`,
			"fixture/mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"local":{"type":"stdio","command":"plugin-fixture","args":["-test.run=^TestPluginMCPProcess$"],"env":{"JAZ_PLUGIN_MCP_PROCESS":"1","VALUE":"` + value + `"}}}}`,
		} {
			file, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(file, content); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/plugins", &archive))
		if recorder.Code != http.StatusOK {
			t.Fatalf("install = %d %s", recorder.Code, recorder.Body)
		}
	}
	install("${PLUGIN_ROOT}")
	proxy := httptest.NewServer(manager.Handler())
	defer proxy.Close()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "agent"}, nil)
	session, err := client.Connect(t.Context(), &mcpsdk.StreamableClientTransport{Endpoint: proxy.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(want string) string {
		t.Helper()
		list, err := session.ListTools(t.Context(), nil)
		if err != nil || len(list.Tools) != 1 {
			t.Fatalf("agent tools = %+v %v", list, err)
		}
		result, err := session.CallTool(t.Context(), &mcpsdk.CallToolParams{Name: list.Tools[0].Name})
		if err != nil || result.IsError {
			t.Fatalf("agent call = %+v %v", result, err)
		}
		var output map[string]string
		if err := json.Unmarshal([]byte(result.Content[0].(*mcpsdk.TextContent).Text), &output); err != nil {
			t.Fatal(err)
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(layout.Plugins, "fixture"))
		if err != nil {
			t.Fatal(err)
		}
		cwd, err := filepath.EvalSymlinks(output["cwd"])
		if err != nil {
			t.Fatal(err)
		}
		if cwd != resolved || output["root"] != filepath.Join(layout.Plugins, "fixture") || output["data"] != filepath.Join(layout.Root, "plugin-data", "fixture") || output["value"] != want {
			t.Fatalf("process configuration = %+v", output)
		}
		return output["pid"]
	}
	first := call(filepath.Join(layout.Plugins, "fixture"))
	install("${PLUGIN_ROOT}")
	if call(filepath.Join(layout.Plugins, "fixture")) == first {
		t.Fatal("replacing a plugin with the same config kept the old process")
	}
	points, err := manager.Entrypoints(t.Context())
	if err != nil || len(points) != 1 {
		t.Fatalf("app entrypoints = %+v %v", points, err)
	}
	html, err := manager.ReadApp(t.Context(), points[0].ServerID, "inspect")
	if err != nil || !strings.Contains(html, "Plugin MCP App") {
		t.Fatalf("MCP App = %s %v", html, err)
	}
	install("updated")
	call("updated")
	stopped := filepath.Join(layout.Root, "plugin-data", "fixture", "stopped")
	if _, err := os.Stat(stopped); err != nil {
		t.Fatal("replaced stdio process did not stop", err)
	}
	if err := os.Remove(stopped); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/v1/plugins/fixture", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("remove = %d %s", recorder.Code, recorder.Body)
	}
	if list, err := session.ListTools(t.Context(), nil); err != nil || len(list.Tools) != 0 {
		t.Fatalf("removed agent tools = %+v %v", list, err)
	}
	if points, err := manager.Entrypoints(t.Context()); err != nil || len(points) != 0 {
		t.Fatalf("removed apps = %+v %v", points, err)
	}
	if _, err := os.Stat(stopped); err != nil {
		t.Fatal("removed stdio process did not stop", err)
	}
}
