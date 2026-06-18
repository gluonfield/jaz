package mcp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

const (
	RegistryGroup  = "mcp"
	maxToolNameLen = 64
)

type Manager struct {
	store    mcpconfig.ServerReader
	tokens   integrationoauth.Store
	registry *tools.Registry
	log      *log.Logger

	localServers map[string]localServer

	mu       sync.RWMutex
	sessions map[string]*serverSession
	statuses map[string]mcpconfig.ServerStatus
	refresh  uint64
}

type Option func(*Manager)

type localServer struct {
	server   mcpconfig.Server
	provider func() *mcpsdk.Server
}

func WithLocalServer(serverID string, server *mcpsdk.Server) Option {
	return WithLocalServerProvider(serverID, func() *mcpsdk.Server { return server })
}

func WithLocalServerProvider(serverID string, provider func() *mcpsdk.Server) Option {
	return WithBuiltinServerProvider(mcpconfig.Server{
		ID:      serverID,
		Name:    serverID,
		Enabled: true,
	}, provider)
}

func WithBuiltinServerProvider(server mcpconfig.Server, provider func() *mcpsdk.Server) Option {
	return func(m *Manager) {
		server.ID = strings.TrimSpace(server.ID)
		if server.ID == "" || provider == nil {
			return
		}
		server.Name = strings.TrimSpace(server.Name)
		if server.Name == "" {
			server.Name = server.ID
		}
		server.Enabled = true
		m.localServers[server.ID] = localServer{server: server, provider: provider}
	}
}

type serverSession struct {
	session      *mcpsdk.ClientSession
	localSession *mcpsdk.ServerSession
	tools        []remoteTool
}

type remoteTool struct {
	serverName  string
	remoteName  string
	description string
	inputSchema map[string]any
	definition  tools.Definition
	session     *mcpsdk.ClientSession
}

type refreshResult struct {
	server  mcpconfig.Server
	session *serverSession
	status  mcpconfig.ServerStatus
}

func NewManager(store mcpconfig.ServerReader, tokens integrationoauth.Store, registry *tools.Registry, logger *log.Logger, opts ...Option) *Manager {
	if logger == nil {
		logger = log.Default()
	}
	m := &Manager{
		store:        store,
		tokens:       tokens,
		registry:     registry,
		log:          logger.WithPrefix("mcp"),
		localServers: make(map[string]localServer),
		sessions:     make(map[string]*serverSession),
		statuses:     make(map[string]mcpconfig.ServerStatus),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// backgroundHandler builds a non-interactive OAuth handler that serves stored
// tokens (refreshing them when possible) but never opens a browser. Returns nil
// when no token store is configured.
func (m *Manager) backgroundHandler(server mcpconfig.Server) *oauthHandler {
	if m.tokens == nil {
		return nil
	}
	return newOAuthHandler(server, m.tokens, http.DefaultClient, m.log)
}

func (m *Manager) Refresh(ctx context.Context) {
	seq := m.beginRefresh()
	servers := m.servers(ctx, nil)
	m.refreshServerList(ctx, seq, servers)
}

func (m *Manager) RefreshLocal(ctx context.Context) {
	seq := m.beginRefresh()
	servers := m.servers(ctx, func(server mcpconfig.Server) bool {
		return m.hasLocalServer(server.ID)
	})
	m.refreshServerList(ctx, seq, servers)
}

func (m *Manager) beginRefresh() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refresh++
	return m.refresh
}

func (m *Manager) servers(ctx context.Context, include func(mcpconfig.Server) bool) []mcpconfig.Server {
	servers, err := m.store.ListMCPServers()
	if err != nil {
		m.log.Error("load mcp servers failed", "error", err)
		servers = nil
	}
	if include != nil {
		filtered := make([]mcpconfig.Server, 0, len(servers))
		for _, server := range servers {
			if include(server) {
				filtered = append(filtered, server)
			}
		}
		servers = filtered
	}
	seen := make(map[string]bool, len(servers)+len(m.localServers))
	for _, server := range servers {
		if strings.TrimSpace(server.ID) != "" {
			seen[strings.TrimSpace(server.ID)] = true
		}
	}
	for _, local := range m.localServers {
		if seen[local.server.ID] {
			continue
		}
		if include == nil || include(local.server) {
			servers = append(servers, local.server)
		}
	}
	return servers
}

func (m *Manager) refreshServerList(ctx context.Context, seq uint64, servers []mcpconfig.Server) {
	results := make([]refreshResult, len(servers))
	var wg sync.WaitGroup
	for i, server := range servers {
		results[i].server = server
		if !server.Enabled {
			results[i].status = mcpconfig.ServerStatus{Status: "disabled"}
			continue
		}
		wg.Add(1)
		go func(index int, server mcpconfig.Server) {
			defer wg.Done()
			sessionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			handler := m.backgroundHandler(server)
			ss, err := m.connect(sessionCtx, server, asOAuthHandler(handler))
			if err != nil {
				results[index].status = connectErrorStatus(handler, err)
				m.log.Warn("mcp server unavailable", "server", server.Name, "error", err)
				return
			}
			results[index].session = ss
			results[index].status = mcpconfig.ServerStatus{Status: "connected", ToolCount: len(ss.tools), CheckedAt: time.Now().UTC()}
		}(i, server)
	}
	wg.Wait()

	next := make(map[string]*serverSession)
	statuses := make(map[string]mcpconfig.ServerStatus)
	var allTools []tools.Tool
	usedNames := map[string]string{}
	for _, result := range results {
		statuses[result.server.ID] = result.status
		if result.session == nil {
			continue
		}
		next[result.server.ID] = result.session
		for i := range result.session.tools {
			rt := result.session.tools[i]
			name := mappedToolName(result.server, rt.remoteName, usedNames)
			rt.definition = tools.Function(name, rt.description, false, rt.inputSchema)
			result.session.tools[i] = rt
			allTools = append(allTools, rt)
		}
	}

	m.mu.Lock()
	if seq != m.refresh {
		m.mu.Unlock()
		closeSessions(next)
		return
	}
	old := m.sessions
	m.sessions = next
	m.statuses = statuses
	m.mu.Unlock()
	m.registry.SetGroup(RegistryGroup, allTools)

	closeSessions(old)
}

func (m *Manager) Close() {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[string]*serverSession)
	m.statuses = make(map[string]mcpconfig.ServerStatus)
	m.mu.Unlock()
	m.registry.RemoveGroup(RegistryGroup)
	closeSessions(sessions)
}

func closeSessions(sessions map[string]*serverSession) {
	for _, ss := range sessions {
		_ = ss.session.Close()
		if ss.localSession != nil {
			_ = ss.localSession.Close()
		}
	}
}

func (m *Manager) Status(id string) mcpconfig.ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statuses[id]
}

func (m *Manager) Test(ctx context.Context, server mcpconfig.Server) mcpconfig.ServerStatus {
	if strings.TrimSpace(server.ID) == "" {
		server.ID = "test"
	}
	sessionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	handler := m.backgroundHandler(server)
	ss, err := m.connect(sessionCtx, server, asOAuthHandler(handler))
	if err != nil {
		return connectErrorStatus(handler, err)
	}
	closeSessions(map[string]*serverSession{server.ID: ss})
	return mcpconfig.ServerStatus{Status: "connected", ToolCount: len(ss.tools), CheckedAt: time.Now().UTC()}
}

// Authorize runs the interactive OAuth authorization-code flow for a server
// (opening the user's browser), persists the resulting token, and reconnects so
// the server's tools become available. It blocks until the user completes sign-in
// or ctx is cancelled.
func (m *Manager) Authorize(ctx context.Context, server mcpconfig.Server) mcpconfig.ServerStatus {
	if m.tokens == nil {
		return mcpconfig.ServerStatus{Status: "error", Error: "token store is not configured", CheckedAt: time.Now().UTC()}
	}
	receiver, err := newLoopbackReceiver()
	if err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}
	defer receiver.close()

	handler := newOAuthHandler(server, m.tokens, http.DefaultClient, m.log)
	handler.interactive = true
	handler.redirectURL = receiver.redirectURL
	handler.fetch = receiver.fetch

	sessionCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	ss, err := m.connect(sessionCtx, server, handler)
	if err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}
	closeSessions(map[string]*serverSession{server.ID: ss})

	// Reconnect everything so this server's tools are installed in the registry
	// using the token we just persisted.
	m.Refresh(context.Background())
	if status := m.Status(server.ID); status.Status != "" {
		return status
	}
	return mcpconfig.ServerStatus{Status: "connected", ToolCount: len(ss.tools), CheckedAt: time.Now().UTC()}
}

// asOAuthHandler converts a possibly-nil *oauthHandler to the interface without
// producing a non-nil interface wrapping a nil pointer.
func asOAuthHandler(h *oauthHandler) auth.OAuthHandler {
	if h == nil {
		return nil
	}
	return h
}

func connectErrorStatus(handler *oauthHandler, err error) mcpconfig.ServerStatus {
	if handler != nil && handler.needsAuthorization() {
		return mcpconfig.ServerStatus{Status: "needs_auth", Error: "Sign in required", CheckedAt: time.Now().UTC()}
	}
	return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
}

func (m *Manager) connect(ctx context.Context, server mcpconfig.Server, handler auth.OAuthHandler) (*serverSession, error) {
	if local := m.localServer(server.ID); local != nil {
		return m.connectLocal(ctx, server, local)
	}
	headers, err := mcpconfig.ResolvedHeaders(server, true)
	if err != nil {
		return nil, err
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "jaz", Version: "0.1.0"}, nil)
	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           &http.Client{Transport: headerTransport{headers: headers, base: http.DefaultTransport}},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
		OAuthHandler:         handler,
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	ss := &serverSession{session: session}
	for _, tool := range list.Tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		rt := remoteTool{
			serverName:  server.Name,
			remoteName:  tool.Name,
			session:     session,
			description: toolDescription(server, tool),
			inputSchema: inputSchema(tool.InputSchema),
		}
		ss.tools = append(ss.tools, rt)
	}
	return ss, nil
}

func (m *Manager) localServer(id string) *mcpsdk.Server {
	local, ok := m.localServers[strings.TrimSpace(id)]
	if !ok || local.provider == nil {
		return nil
	}
	return local.provider()
}

func (m *Manager) hasLocalServer(id string) bool {
	local, ok := m.localServers[strings.TrimSpace(id)]
	return ok && local.provider != nil
}

func (m *Manager) connectLocal(ctx context.Context, server mcpconfig.Server, local *mcpsdk.Server) (*serverSession, error) {
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	localSession, err := local.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, err
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "jaz", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = localSession.Close()
		return nil, err
	}
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		_ = session.Close()
		_ = localSession.Close()
		return nil, err
	}
	ss := &serverSession{session: session, localSession: localSession}
	for _, tool := range list.Tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		rt := remoteTool{
			serverName:  server.Name,
			remoteName:  tool.Name,
			session:     session,
			description: toolDescription(server, tool),
			inputSchema: inputSchema(tool.InputSchema),
		}
		ss.tools = append(ss.tools, rt)
	}
	return ss, nil
}

type headerTransport struct {
	headers []mcpconfig.Header
	base    http.RoundTripper
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for _, header := range t.headers {
		if strings.TrimSpace(header.Name) == "" {
			continue
		}
		clone.Header.Set(header.Name, header.Value)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func (t remoteTool) Definition() tools.Definition {
	return t.definition
}

func (t remoteTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	res, err := t.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      t.remoteName,
		Arguments: inputs,
	})
	if err != nil {
		return tools.Result{}, err
	}
	status := "completed"
	if res.IsError {
		status = "error"
	}
	content := make([]any, 0, len(res.Content))
	for _, item := range res.Content {
		data, err := item.MarshalJSON()
		if err != nil {
			return tools.Result{}, err
		}
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			return tools.Result{}, err
		}
		content = append(content, decoded)
	}
	return tools.JSONResult(map[string]any{
		"status":             status,
		"server":             t.serverName,
		"tool":               t.remoteName,
		"content":            content,
		"structured_content": res.StructuredContent,
	})
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func mappedToolName(server mcpconfig.Server, remote string, used map[string]string) string {
	source := server.ID + ":" + remote
	base := sanitizeToolName("mcp_" + server.Name + "_" + remote)
	name := clampToolName(base, source)
	if existing, ok := used[name]; ok && existing != source {
		name = clampToolName(base+"_"+shortHash(source), source)
	}
	used[name] = source
	return name
}

func sanitizeToolName(value string) string {
	value = unsafeName.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_-")
	if value == "" {
		return "mcp_tool"
	}
	if len(value) > 0 && value[0] >= '0' && value[0] <= '9' {
		value = "mcp_" + value
	}
	return value
}

func clampToolName(value, source string) string {
	if len(value) <= maxToolNameLen {
		return value
	}
	suffix := "_" + shortHash(source)
	keep := maxToolNameLen - len(suffix)
	if keep < 1 {
		return strings.TrimPrefix(suffix, "_")
	}
	return strings.TrimRight(value[:keep], "_-") + suffix
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func toolDescription(server mcpconfig.Server, tool *mcpsdk.Tool) string {
	desc := strings.TrimSpace(tool.Description)
	if desc == "" {
		return "MCP tool from " + server.Name + "."
	}
	return "MCP tool from " + server.Name + ": " + desc
}

func inputSchema(schema any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object"}
	}
	data, err := json.Marshal(schema)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return map[string]any{"type": "object"}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil || len(out) == 0 {
		return map[string]any{"type": "object"}
	}
	if _, ok := out["type"]; !ok {
		out["type"] = "object"
	}
	return out
}
