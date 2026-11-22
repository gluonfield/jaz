package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testgit"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestPluginMCPProcess(t *testing.T) {
	if os.Getenv("JAZ_PLUGIN_MCP_PROCESS") != "1" {
		return
	}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "plugin-fixture"}, nil)
	if gate := os.Getenv("GATE_FILE"); gate != "" {
		server.AddReceivingMiddleware(func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
			return func(ctx context.Context, method string, request mcpsdk.Request) (mcpsdk.Result, error) {
				if method == "tools/list" {
					_ = os.WriteFile(gate+".entered", nil, 0600)
					ticker := time.NewTicker(10 * time.Millisecond)
					defer ticker.Stop()
					for {
						if _, err := os.Stat(gate); err == nil {
							break
						}
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case <-ticker.C:
						}
					}
				}
				return next(ctx, method, request)
			}
		})
	}
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
	db, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := plugins.New(layout)
	service := plugins.NewService(store, db, nil, nil, nil, nil)
	unrelated := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "unrelated"}, nil)
	unrelated.AddTool(&mcpsdk.Tool{Name: "unrelated", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{}, nil
	})
	var blocked atomic.Bool
	var requests atomic.Int32
	release := make(chan struct{})
	remoteHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return unrelated }, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if blocked.Load() {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		remoteHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	unrelatedRepo := makeRepo(t, map[string]string{
		"unrelated/plugin.json": `{"name":"unrelated"}`,
		"unrelated/mcp.json":    `{"mcpServers":{"remote":{"type":"streamable-http","url":"` + upstream.URL + `"}}}`,
	})
	if _, err := store.InstallRepository(t.Context(), plugins.RepositoryInput{Repository: unrelatedRepo}); err != nil {
		t.Fatal(err)
	}
	manager := mcpruntime.NewManager(service, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	defer close(release)
	manager.Refresh(t.Context())
	blocked.Store(true)
	before := requests.Load()
	until := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !check() {
			if time.Now().After(deadline) {
				t.Fatal("background plugin refresh did not settle")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	handler := NewHandler(service, sessionStore{}, manager, "")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/plugins/install", handler.Install)
	mux.HandleFunc("DELETE /v1/plugins/{plugin}", handler.Remove)
	mux.HandleFunc("POST /v1/plugins/{plugin}/update", handler.Update)
	var previousRevision uint64
	request := func(method, path string, body io.Reader) *httptest.ResponseRecorder {
		t.Helper()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		response := httptest.NewRecorder()
		done := make(chan struct{})
		start := time.Now()
		previousRevision = manager.Revision()
		go func() {
			mux.ServeHTTP(response, httptest.NewRequest(method, path, body).WithContext(ctx))
			close(done)
		}()
		select {
		case <-done:
			t.Logf("%s %s response: %s", method, path, time.Since(start))
		case <-time.After(2 * time.Second):
			t.Fatal("plugin response waited for MCP discovery")
		}
		return response
	}
	connected := func() {
		until(func() bool {
			return manager.Revision() > previousRevision && manager.Status(plugins.MCPServerID("fixture", "local")).Status == "connected"
		})
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-c", "core.hooksPath=" + os.DevNull}, args...)...)
		command.Dir = repo
		data, err := command.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(data))
	}
	git("init", "--quiet")
	writeConfig := func(value string, gate bool) {
		t.Helper()
		env := `{"JAZ_PLUGIN_MCP_PROCESS":"1","VALUE":"` + value + `"}`
		if gate {
			env = `{"JAZ_PLUGIN_MCP_PROCESS":"1","GATE_FILE":"${PLUGIN_DATA}/allow","VALUE":"` + value + `"}`
		}
		for name, body := range map[string]string{
			"plugin.json": `{"name":"fixture"}`,
			"mcp.json":    `{"mcpServers":{"local":{"type":"stdio","command":"plugin-fixture","cwd":"${PLUGIN_DATA}","args":["-test.run=^TestPluginMCPProcess$"],"env":` + env + `}}}`,
		} {
			if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
		}
		git("add", "--all")
		testgit.Snapshot(t, repo)
	}
	writeConfig("${PLUGIN_ROOT}", true)
	input, _ := json.Marshal(plugins.RepositoryInput{Repository: repo})
	recorder := request(http.MethodPost, "/v1/plugins/install", bytes.NewReader(input))
	if recorder.Code != http.StatusOK {
		t.Fatalf("install = %d %s", recorder.Code, recorder.Body)
	}
	gate := filepath.Join(layout.Root, "plugin-data", "fixture", "allow")
	until(func() bool { _, err := os.Stat(gate + ".entered"); return err == nil })
	if manager.Status(plugins.MCPServerID("fixture", "local")).Status == "connected" {
		t.Fatal("MCP connected before its fixture was released")
	}
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
	connected()

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
		if err != nil || len(list.Tools) != 2 {
			t.Fatalf("agent tools = %+v %v", list, err)
		}
		name := ""
		for _, tool := range list.Tools {
			if strings.HasSuffix(tool.Name, "inspect") {
				name = tool.Name
			}
		}
		result, err := session.CallTool(t.Context(), &mcpsdk.CallToolParams{Name: name})
		if err != nil || result.IsError {
			t.Fatalf("agent call = %+v %v", result, err)
		}
		var output map[string]string
		if err := json.Unmarshal([]byte(result.Content[0].(*mcpsdk.TextContent).Text), &output); err != nil {
			t.Fatal(err)
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(layout.Root, "plugin-data", "fixture"))
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
	points, err := manager.Entrypoints(t.Context())
	if err != nil || len(points) != 1 {
		t.Fatalf("app entrypoints = %+v %v", points, err)
	}
	html, err := manager.ReadApp(t.Context(), points[0].ServerID, "inspect")
	if err != nil || !strings.Contains(html, "Plugin MCP App") {
		t.Fatalf("MCP App = %s %v", html, err)
	}
	writeConfig("repo-second", false)
	response := request(http.MethodPost, "/v1/plugins/fixture/update", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("repository update = %d %s", response.Code, response.Body)
	}
	connected()
	if call("repo-second") == first {
		t.Fatal("updating plugin kept old process")
	}
	listed, err := store.List()
	if err != nil || listed[0].Source.Commit != git("rev-parse", "HEAD") {
		t.Fatalf("updated source = %+v %v", listed, err)
	}
	stopped := filepath.Join(layout.Root, "plugin-data", "fixture", "stopped")
	if _, err := os.Stat(stopped); err != nil {
		t.Fatal("replaced stdio process did not stop", err)
	}
	if err := os.Remove(stopped); err != nil {
		t.Fatal(err)
	}
	recorder = request(http.MethodDelete, "/v1/plugins/fixture", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("remove = %d %s", recorder.Code, recorder.Body)
	}
	until(func() bool { _, err := os.Stat(stopped); return err == nil })
	if list, err := session.ListTools(t.Context(), nil); err != nil || len(list.Tools) != 1 {
		t.Fatalf("removed agent tools = %+v %v", list, err)
	}
	if points, err := manager.Entrypoints(t.Context()); err != nil || len(points) != 0 {
		t.Fatalf("removed apps = %+v %v", points, err)
	}
	if requests.Load() != before {
		t.Fatalf("plugin mutation contacted unrelated server: %d -> %d", before, requests.Load())
	}
}
