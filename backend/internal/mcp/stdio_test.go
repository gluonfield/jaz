//go:build !windows

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestMCPStdioProcess(t *testing.T) {
	if os.Getenv("JAZ_MCP_TEST_PROCESS") != "1" {
		return
	}
	dir := os.Getenv("JAZ_MCP_TEST_DIR")
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(filepath.Join(dir, "started-"+pid), nil, 0o600); err != nil {
		os.Exit(2)
	}
	if os.Getenv("JAZ_MCP_TEST_HANG") == "1" {
		time.Sleep(time.Hour)
		os.Exit(2)
	}
	if os.Getenv("JAZ_MCP_TEST_CHILD") == "1" {
		child := exec.Command(os.Args[0], "-test.run=^TestMCPStdioProcess$")
		child.Env = append(os.Environ(), "JAZ_MCP_TEST_HANG=1")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		_ = os.WriteFile(filepath.Join(dir, "child"), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
	}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "stdio-test"}, nil)
	server.AddTool(&mcpsdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		if strings.Contains(string(req.Params.Arguments), "crash") {
			os.Exit(3)
		}
		data, _ := json.Marshal(map[string]any{"pid": pid, "args": os.Args[3:], "value": os.Getenv("JAZ_MCP_TEST_VALUE"), "inherited": os.Getenv("JAZ_MCP_TEST_INHERITED")})
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}}}, nil
	})
	_ = server.Run(context.Background(), &mcpsdk.StdioTransport{})
	_ = os.WriteFile(filepath.Join(dir, "stopped-"+pid), nil, 0o600)
	os.Exit(0)
}

func TestStdioShutdownTerminatesDescendants(t *testing.T) {
	server, dir := stdioTestServer(t)
	server.Env["JAZ_MCP_TEST_CHILD"] = "1"
	manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	manager.Refresh(t.Context())
	if manager.Status(server.ID).Status != "connected" {
		t.Fatal("server with child did not connect")
	}
	data, err := os.ReadFile(filepath.Join(dir, "child"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	manager.Close()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("local server left a descendant process running")
}

func stdioTestServer(t *testing.T) (mcpconfig.Server, string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "server with spaces")
	if err := os.Symlink(binary, command); err != nil {
		t.Fatal(err)
	}
	return mcpconfig.Server{
		ID: "local", Name: "Local", Transport: mcpconfig.TransportStdio, Enabled: true,
		Command: command, Args: []string{"-test.run=^TestMCPStdioProcess$", "--", "argument with spaces", "$(literal); no shell"},
		Env: map[string]string{"JAZ_MCP_TEST_PROCESS": "1", "JAZ_MCP_TEST_DIR": dir, "JAZ_MCP_TEST_VALUE": "first"},
	}, dir
}

func TestStdioToolsThroughProxyAndProcessLifecycle(t *testing.T) {
	t.Setenv("JAZ_MCP_TEST_INHERITED", "inherited")
	server, dir := stdioTestServer(t)
	store := &testStore{servers: []mcpconfig.Server{server}}
	manager := NewManager(store, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	manager.Refresh(ctx)
	cancel()
	if status := manager.Status(server.ID); status.Status != "connected" || status.ToolCount != 1 {
		t.Fatalf("local status = %#v", status)
	}
	proxy := httptest.NewServer(manager.Handler())
	defer proxy.Close()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "agent"}, nil)
	session, err := client.Connect(t.Context(), &mcpsdk.StreamableClientTransport{Endpoint: proxy.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(t.Context(), nil)
	if err != nil || len(list.Tools) != 1 {
		t.Fatalf("proxy catalog = %#v, %v", list, err)
	}
	call := func() string {
		t.Helper()
		result, err := session.CallTool(t.Context(), &mcpsdk.CallToolParams{Name: list.Tools[0].Name})
		if err != nil || result.IsError {
			t.Fatalf("proxy call = %#v, %v", result, err)
		}
		var output struct {
			PID       string   `json:"pid"`
			Args      []string `json:"args"`
			Value     string   `json:"value"`
			Inherited string   `json:"inherited"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcpsdk.TextContent).Text), &output); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(output.Args, server.Args[2:]) || output.Value != store.servers[0].Env["JAZ_MCP_TEST_VALUE"] || output.Inherited != "inherited" {
			t.Fatalf("command inputs changed: %#v", output)
		}
		return output.PID
	}
	first := call()
	manager.Refresh(t.Context())
	if call() != first {
		t.Fatal("unchanged configuration restarted the process")
	}
	store.servers[0].Env["JAZ_MCP_TEST_VALUE"] = "updated"
	manager.Refresh(t.Context())
	second := call()
	if second == first {
		t.Fatal("changed environment did not restart the process")
	}
	if _, err := os.Stat(filepath.Join(dir, "stopped-"+first)); err != nil {
		t.Fatal("replaced process did not exit", err)
	}
	store.servers[0].Enabled = false
	manager.Refresh(t.Context())
	if _, err := os.Stat(filepath.Join(dir, "stopped-"+second)); err != nil {
		t.Fatal("disabled process did not exit", err)
	}
	if list, err := session.ListTools(t.Context(), nil); err != nil || len(list.Tools) != 0 {
		t.Fatalf("disabled tools remain: %#v, %v", list, err)
	}
	store.servers[0].Enabled = true
	manager.Refresh(t.Context())
	third := call()
	store.servers = nil
	manager.Refresh(t.Context())
	if _, err := os.Stat(filepath.Join(dir, "stopped-"+third)); err != nil {
		t.Fatal("deleted process did not exit", err)
	}
	store.servers = []mcpconfig.Server{server}
	manager.Refresh(t.Context())
	fourth := call()
	manager.Close()
	if _, err := os.Stat(filepath.Join(dir, "stopped-"+fourth)); err != nil {
		t.Fatal("manager shutdown did not stop process", err)
	}
}

func TestStdioFailureDoesNotReplayToolAndRefreshRecovers(t *testing.T) {
	server, dir := stdioTestServer(t)
	manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	manager.Refresh(t.Context())
	connection := manager.sessions[server.ID].serverConnection
	if _, err := connection.callTool(t.Context(), &mcpsdk.CallToolParams{Name: "echo", Arguments: map[string]any{"crash": true}}); err == nil {
		t.Fatal("crashed process reported success")
	}
	entries, err := filepath.Glob(filepath.Join(dir, "started-*"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("tool was replayed in a new process: %v, %v", entries, err)
	}
	manager.Refresh(t.Context())
	if manager.Status(server.ID).Status != "connected" || manager.sessions[server.ID].serverConnection == connection {
		t.Fatal("refresh did not recover the dead process")
	}
}

func TestStdioConnectionErrorsAndDiscoveryCleanup(t *testing.T) {
	server, dir := stdioTestServer(t)
	manager := NewManager(&testStore{}, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	if status := manager.Test(t.Context(), server); status.Status != "connected" {
		t.Fatalf("test connection = %#v", status)
	}
	started, _ := filepath.Glob(filepath.Join(dir, "started-*"))
	stopped, _ := filepath.Glob(filepath.Join(dir, "stopped-*"))
	if len(started) != 1 || len(stopped) != 1 {
		t.Fatal("test connection leaked its process")
	}
	server.Command = filepath.Join(dir, "missing")
	if status := manager.Test(t.Context(), server); status.Status != "error" || !strings.Contains(status.Error, "connect local MCP server") {
		t.Fatalf("missing executable status = %#v", status)
	}
	server, _ = stdioTestServer(t)
	server.Env["JAZ_MCP_TEST_HANG"] = "1"
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	if status := manager.Test(ctx, server); status.Status != "error" {
		t.Fatalf("hung startup status = %#v", status)
	}
	if time.Since(startedAt) > 6*time.Second {
		t.Fatal("hung process shutdown exceeded its bound")
	}
	started, err := filepath.Glob(filepath.Join(server.Env["JAZ_MCP_TEST_DIR"], "started-*"))
	if err != nil || len(started) != 1 {
		t.Fatalf("hung process did not start: %v, %v", started, err)
	}
	pid, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(started[0]), "started-"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("hung process is still alive or unreaped: %v", err)
	}
}
