package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/integrationingest"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type testStore struct {
	servers []mcpconfig.Server
}

func (s *testStore) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), s.servers...), nil
}

func TestCallbackReceiverSurfacesAuthURLAndCompletesFromCallback(t *testing.T) {
	manager := NewManager(&testStore{}, newMemTokenStore(), tools.NewRegistry(), log.New(io.Discard))
	started := make(chan authorizationStart, 1)
	receiver := &callbackReceiver{
		manager:     manager,
		redirectURL: "https://jaz.example.com/v1/mcp/oauth/callback",
		started:     started,
		pending:     newAuthorizationPending(),
	}
	authURL := "https://auth.example.com/authorize?client_id=jaz&state=state-1"
	done := make(chan struct {
		code  string
		state string
		err   error
	}, 1)
	go func() {
		code, state, err := receiver.fetch(context.Background(), authURL)
		done <- struct {
			code  string
			state string
			err   error
		}{code: code, state: state, err: err}
	}()

	select {
	case start := <-started:
		if start.authURL != authURL {
			t.Fatalf("auth url = %q, want %q", start.authURL, authURL)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for auth url")
	}
	callbackDone := make(chan error, 1)
	go func() {
		callbackDone <- manager.CompleteAuthorization(context.Background(), "state-1", "code-1", "")
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("fetch err = %v", got.err)
		}
		if got.code != "code-1" || got.state != "state-1" {
			t.Fatalf("fetch = code %q, state %q", got.code, got.state)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for callback result")
	}
	select {
	case err := <-callbackDone:
		t.Fatalf("CompleteAuthorization returned before token exchange completed: %v", err)
	default:
	}
	receiver.pending.complete(mcpconfig.ServerStatus{Status: "connected"})
	select {
	case err := <-callbackDone:
		if err != nil {
			t.Fatalf("CompleteAuthorization: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for authorization completion")
	}
	if _, ok := manager.takeAuthorizationState("state-1"); ok {
		t.Fatal("authorization state was not cleaned up")
	}
}

func TestCompleteAuthorizationReturnsFinalFailureStatus(t *testing.T) {
	manager := NewManager(&testStore{}, newMemTokenStore(), tools.NewRegistry(), log.New(io.Discard))
	started := make(chan authorizationStart, 1)
	receiver := &callbackReceiver{
		manager:     manager,
		redirectURL: "https://jaz.example.com/v1/mcp/oauth/callback",
		started:     started,
		pending:     newAuthorizationPending(),
	}
	done := make(chan error, 1)
	go func() {
		_, _, _ = receiver.fetch(context.Background(), "https://auth.example.com/authorize?state=state-fail")
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for auth url")
	}
	go func() {
		done <- manager.CompleteAuthorization(context.Background(), "state-fail", "code-1", "")
	}()
	receiver.pending.complete(mcpconfig.ServerStatus{Status: "error", Error: "token exchange failed"})
	select {
	case err := <-done:
		if err == nil || err.Error() != "token exchange failed" {
			t.Fatalf("CompleteAuthorization error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for authorization failure")
	}
}

func TestAuthorizationHookFailureRollsBackToken(t *testing.T) {
	store := newMemTokenStore()
	tokenID := mcpconfig.OAuthConnectionID("srv-hook")
	if err := store.SaveToken(context.Background(), tokenID, integrationoauth.Token{AccessToken: "access"}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(&testStore{}, store, tools.NewRegistry(), log.New(io.Discard))
	err := manager.runAuthorizationHook(context.Background(), "srv-hook", func(context.Context) error {
		return errors.New("save connection")
	})
	if err == nil || !strings.Contains(err.Error(), "save connection") {
		t.Fatalf("err = %v", err)
	}
	if _, ok, err := store.LoadToken(context.Background(), tokenID); err != nil || ok {
		t.Fatalf("token after failed hook ok=%v err=%v", ok, err)
	}
}

type echoInput struct {
	Value string `json:"value"`
}

func TestManagerRefreshMapsAndExecutesRemoteTools(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-mcp", Version: "1.0.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "echo",
		Description: "echoes a value",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "got " + input.Value}},
		}, map[string]string{"value": input.Value}, nil
	})

	handler := mcpsdk.NewStreamableHTTPHandler(func(req *http.Request) *mcpsdk.Server {
		if req.Header.Get("X-Test") != "secret" {
			return nil
		}
		return server
	}, &mcpsdk.StreamableHTTPOptions{JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	registry := tools.NewRegistry()
	manager := NewManager(&testStore{servers: []mcpconfig.Server{{
		ID:        "srv1",
		Name:      "Remote Test",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       httpServer.URL,
		Enabled:   true,
		Headers:   []mcpconfig.Header{{Name: "X-Test", Value: "secret"}},
	}}}, nil, registry, log.New(io.Discard))
	manager.Refresh(context.Background())
	defer manager.Close()

	status := manager.Status("srv1")
	if status.Status != "connected" || status.ToolCount != 1 {
		t.Fatalf("status = %#v", status)
	}
	defs := registry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("registry definitions = %d, want 1", len(defs))
	}
	name := tools.DefinitionName(defs[0])
	if !strings.HasPrefix(name, "mcp_Remote_Test_echo") {
		t.Fatalf("tool name = %q", name)
	}
	if len(status.Tools) != 1 || status.Tools[0].Name != name ||
		status.Tools[0].RemoteName != "echo" ||
		!strings.Contains(status.Tools[0].Description, "echoes a value") {
		t.Fatalf("status tools = %#v, mapped name = %q", status.Tools, name)
	}
	tool, ok := registry.Get(name)
	if !ok {
		t.Fatalf("tool %q not registered", name)
	}
	result, err := tool.Execute(context.Background(), map[string]any{"value": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Status            string            `json:"status"`
		Server            string            `json:"server"`
		Tool              string            `json:"tool"`
		Content           []map[string]any  `json:"content"`
		StructuredContent map[string]string `json:"structured_content"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "completed" || payload.Server != "Remote Test" || payload.Tool != "echo" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload.Content) != 1 || payload.Content[0]["text"] != "got ok" {
		t.Fatalf("content = %#v", payload.Content)
	}
	if payload.StructuredContent["value"] != "ok" {
		t.Fatalf("structured_content = %#v", payload.StructuredContent)
	}
}

func TestManagerRefreshGatesOAuthConfiguredServerWithoutToken(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-mcp", Version: "1.0.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_labels"}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{}, nil, nil
	})

	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(req *http.Request) *mcpsdk.Server {
		return server
	}, &mcpsdk.StreamableHTTPOptions{JSONResponse: true}))
	defer httpServer.Close()

	registry := tools.NewRegistry()
	manager := NewManager(&testStore{servers: []mcpconfig.Server{{
		ID:        "gmail",
		Name:      "Gmail",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       httpServer.URL,
		Enabled:   true,
		OAuth:     mcpconfig.OAuthConfig{ClientID: "google-client"},
	}}}, newMemTokenStore(), registry, log.New(io.Discard))
	manager.Refresh(context.Background())
	defer manager.Close()

	status := manager.Status("gmail")
	if status.Status != "needs_auth" || status.ToolCount != 1 {
		t.Fatalf("status = %#v", status)
	}
	if defs := registry.Definitions(); len(defs) != 0 {
		t.Fatalf("registry definitions = %d, want 0", len(defs))
	}
}

func TestManagerProxyHandlerExposesSafeRemoteTools(t *testing.T) {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "remote-mcp", Version: "1.0.0"}, nil)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{
		Name:        "repo/search:v1",
		Description: "searches repos",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "proxied " + input.Value}},
		}, map[string]string{"value": input.Value}, nil
	})
	local := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "local-mcp", Version: "1.0.0"}, nil)
	mcpsdk.AddTool(local, &mcpsdk.Tool{
		Name: "local_tool",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{}, map[string]string{"value": input.Value}, nil
	})

	remoteHTTP := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(req *http.Request) *mcpsdk.Server {
		return remote
	}, &mcpsdk.StreamableHTTPOptions{JSONResponse: true}))
	defer remoteHTTP.Close()

	registry := tools.NewRegistry()
	manager := NewManager(&testStore{servers: []mcpconfig.Server{{
		ID:        "remote",
		Name:      "Remote Docs",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       remoteHTTP.URL,
		Enabled:   true,
	}}}, nil, registry, log.New(io.Discard), WithBuiltinServerProvider(mcpconfig.Server{
		ID:      "local",
		Name:    "local",
		Enabled: true,
	}, func() *mcpsdk.Server { return local }))
	defer manager.Close()

	proxyHTTP := httptest.NewServer(manager.Handler())
	defer proxyHTTP.Close()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint:             proxyHTTP.URL,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 1 {
		t.Fatalf("proxy tools = %d, want only remote user tools", len(list.Tools))
	}
	name := list.Tools[0].Name
	if !strings.Contains(name, "mcp_Remote_Docs_repo_search_v1") {
		t.Fatalf("tool name = %q", name)
	}
	for _, ch := range name {
		if !(ch == '_' || ch == '-' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9') {
			t.Fatalf("unsafe character %q in %q", ch, name)
		}
	}

	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: map[string]any{"value": "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v", result.Content)
	}
	text, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok || text.Text != "proxied ok" {
		t.Fatalf("content = %#v", result.Content)
	}
}

func TestManagerProxyHandlerIsStable(t *testing.T) {
	manager := NewManager(&testStore{}, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()

	if manager.Handler() != manager.Handler() {
		t.Fatal("proxy handler must be stable so streamable MCP sessions survive across requests")
	}
}

func TestManagerRefreshLocalCanUseLocalServer(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "local-mcp", Version: "1.0.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "echo",
		Description: "echoes a value",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "local " + input.Value}},
		}, map[string]string{"value": input.Value}, nil
	})

	registry := tools.NewRegistry()
	manager := NewManager(&testStore{servers: []mcpconfig.Server{{
		ID:        "jaztools",
		Name:      "jaztools",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       "http://127.0.0.1:1/mcp/jaztools",
		Enabled:   true,
	}, {
		ID:        "remote",
		Name:      "Remote",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       "http://127.0.0.1:1/mcp/remote",
		Enabled:   true,
	}}}, nil, registry, log.New(io.Discard), WithLocalServer("jaztools", server))
	manager.RefreshLocal(context.Background())
	defer manager.Close()

	status := manager.Status("jaztools")
	if status.Status != "connected" || status.ToolCount != 1 {
		t.Fatalf("status = %#v", status)
	}
	if status := manager.Status("remote"); status.Status != "" {
		t.Fatalf("remote status = %#v, want unset", status)
	}
	tool, ok := registry.Get("echo")
	if !ok {
		t.Fatalf("local handler tool not registered")
	}
	result, err := tool.Execute(context.Background(), map[string]any{"value": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "local ok") {
		t.Fatalf("result = %s", result.Content)
	}
}

func TestManagerRefreshLocalRegistersJaztoolsGmailTools(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "jaztools", Version: "test"}, nil)
	connections.NewGmailMCPTools(nil, integrationingest.RawWriter{Root: t.TempDir()}).AddTo(server)

	registry := tools.NewRegistry()
	manager := NewManager(&testStore{}, nil, registry, log.New(io.Discard), WithBuiltinServerProvider(mcpconfig.Server{
		ID:      "jaztools",
		Name:    "jaztools",
		Enabled: true,
	}, func() *mcpsdk.Server { return server }))
	manager.RefreshLocal(context.Background())
	defer manager.Close()

	status := manager.Status("jaztools")
	if status.Status != "connected" || status.ToolCount != 9 {
		t.Fatalf("status = %#v", status)
	}
	for _, name := range []string{
		"gmail_get_profile",
		"gmail_search_threads",
		"gmail_read_thread",
		"gmail_create_draft",
		"gmail_create_reply_draft",
		"gmail_send_draft",
		"gmail_update_draft",
		"gmail_list_drafts",
		"gmail_read_attachment",
	} {
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("registry missing %s", name)
		}
	}
}

func TestManagerRefreshIncludesBuiltinLocalServerOutsideStore(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "builtin-mcp", Version: "1.0.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "echo",
		Description: "echoes a value",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "builtin " + input.Value}},
		}, map[string]string{"value": input.Value}, nil
	})

	registry := tools.NewRegistry()
	manager := NewManager(&testStore{}, nil, registry, log.New(io.Discard), WithBuiltinServerProvider(mcpconfig.Server{
		ID:      "jaztools",
		Name:    "jaztools",
		Enabled: true,
	}, func() *mcpsdk.Server { return server }))
	manager.Refresh(context.Background())
	defer manager.Close()

	status := manager.Status("jaztools")
	if status.Status != "connected" || status.ToolCount != 1 {
		t.Fatalf("status = %#v", status)
	}
	tool, ok := registry.Get("echo")
	if !ok {
		t.Fatalf("builtin tool not registered")
	}
	result, err := tool.Execute(context.Background(), map[string]any{"value": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "builtin ok") {
		t.Fatalf("result = %s", result.Content)
	}
}

func TestLocalServerProviderIsLazy(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "local-mcp", Version: "1.0.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "echo",
		Description: "echoes a value",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{}, map[string]string{"value": input.Value}, nil
	})

	calls := 0
	registry := tools.NewRegistry()
	manager := NewManager(&testStore{servers: []mcpconfig.Server{{
		ID:        "jaztools",
		Name:      "jaztools",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       "http://127.0.0.1:1/mcp/jaztools",
		Enabled:   true,
	}}}, nil, registry, log.New(io.Discard), WithLocalServerProvider("jaztools", func() *mcpsdk.Server {
		calls++
		return server
	}))
	if calls != 0 {
		t.Fatalf("provider called during manager construction")
	}
	manager.RefreshLocal(context.Background())
	defer manager.Close()
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}

func TestResolvedHeadersUsesEnvAndBearerToken(t *testing.T) {
	t.Setenv("MCP_HEADER_VALUE", "from-env")
	t.Setenv("MCP_TOKEN", "token")
	headers, err := mcpconfig.ResolvedHeaders(mcpconfig.Server{
		Headers:           []mcpconfig.Header{{Name: "X-Literal", Value: "literal"}, {Name: "X-Env", EnvVar: "MCP_HEADER_VALUE"}},
		BearerTokenEnvVar: "MCP_TOKEN",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, header := range headers {
		got[header.Name] = header.Value
	}
	if got["X-Literal"] != "literal" || got["X-Env"] != "from-env" || got["Authorization"] != "Bearer token" {
		t.Fatalf("headers = %#v", got)
	}
}

func TestMappedToolNameIsSafeAndBounded(t *testing.T) {
	used := map[string]string{}
	name := mappedToolName(mcpconfig.Server{ID: "server", Name: "Server With Spaces"}, strings.Repeat("tool/", 40), used)
	if len(name) > maxToolNameLen {
		t.Fatalf("tool name length = %d, want <= %d", len(name), maxToolNameLen)
	}
	for _, ch := range name {
		if !(ch == '_' || ch == '-' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9') {
			t.Fatalf("unsafe character %q in %q", ch, name)
		}
	}
}

type nativeStubTool struct{ name string }

func (t nativeStubTool) Definition() tools.Definition {
	return tools.Function(t.name, "native "+t.name, false, nil)
}

func (t nativeStubTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	return tools.Result{Content: "native"}, nil
}

func TestBuiltinServerToolsUseBareNamesAndYieldToNativeTools(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "jaztools", Version: "test"}, nil)
	for _, name := range []string{"memory_search", "agent_spawn"} {
		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        name,
			Description: name,
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "builtin"}}}, nil, nil
		})
	}

	registry := tools.NewRegistry(nativeStubTool{name: "agent_spawn"})
	manager := NewManager(&testStore{}, nil, registry, log.New(io.Discard), WithBuiltinServerProvider(mcpconfig.Server{
		ID:      "jaztools",
		Name:    "jaztools",
		Enabled: true,
	}, func() *mcpsdk.Server { return server }))
	manager.RefreshLocal(context.Background())

	if _, ok := registry.Get("memory_search"); !ok {
		t.Fatal("builtin tool not registered under bare name")
	}
	if !registry.InGroup(BuiltinRegistryGroup, "memory_search") {
		t.Fatal("builtin tool not in builtin group")
	}
	if registry.InGroup(RegistryGroup, "memory_search") {
		t.Fatal("builtin tool leaked into mcp group")
	}
	if registry.InGroup(BuiltinRegistryGroup, "agent_spawn") {
		t.Fatal("builtin duplicate clobbered the native tool")
	}
	if _, ok := registry.Get("mcp_jaztools_agent_spawn"); ok {
		t.Fatal("builtin tool registered under mcp-prefixed name")
	}

	manager.RefreshLocal(context.Background())
	if _, ok := registry.Get("memory_search"); !ok {
		t.Fatal("builtin tool lost on second refresh")
	}

	manager.Close()
	if _, ok := registry.Get("agent_spawn"); !ok {
		t.Fatal("native tool removed by manager close")
	}
	if _, ok := registry.Get("memory_search"); ok {
		t.Fatal("builtin group not removed by manager close")
	}
}

func TestBuiltinServersDoNotShareToolNames(t *testing.T) {
	newServer := func(reply string) *mcpsdk.Server {
		server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: reply, Version: "test"}, nil)
		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        "echo",
			Description: reply,
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, map[string]string, error) {
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: reply}}}, nil, nil
		})
		return server
	}
	first := newServer("first")
	second := newServer("second")

	registry := tools.NewRegistry()
	manager := NewManager(&testStore{}, nil, registry, log.New(io.Discard), WithBuiltinServerProvider(mcpconfig.Server{
		ID:      "alpha",
		Name:    "alpha",
		Enabled: true,
	}, func() *mcpsdk.Server { return first }), WithBuiltinServerProvider(mcpconfig.Server{
		ID:      "beta",
		Name:    "beta",
		Enabled: true,
	}, func() *mcpsdk.Server { return second }))
	manager.RefreshLocal(context.Background())
	defer manager.Close()

	if _, ok := registry.Get("echo"); !ok {
		t.Fatal("echo not registered")
	}
	count := 0
	for _, def := range registry.Definitions() {
		if tools.DefinitionName(def) == "echo" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("echo registered %d times, want 1", count)
	}
	if manager.Status("alpha").ToolCount+manager.Status("beta").ToolCount != 1 {
		t.Fatalf("statuses = %#v %#v, want one tool total", manager.Status("alpha"), manager.Status("beta"))
	}
}
