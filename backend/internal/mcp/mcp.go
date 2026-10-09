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
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/tools"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

const (
	RegistryGroup        = "mcp"
	BuiltinRegistryGroup = "builtin"
	maxToolNameLen       = 64
	remoteStatusTimeout  = 25 * time.Second

	ProxyServerID   = "jaz_mcp"
	ProxyServerName = "jaz_mcp"
)

func ProxyServerConfig(url string) mcpconfig.Server {
	return mcpconfig.Server{
		ID:        ProxyServerID,
		Name:      ProxyServerName,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       strings.TrimSpace(url),
		Enabled:   true,
		Headers: []mcpconfig.Header{{
			Name:  mcpsession.HeaderName,
			Value: mcpsession.HeaderPlaceholder,
		}},
	}
}

type Manager struct {
	store    mcpconfig.ServerReader
	tokens   tokenStore
	registry *tools.Registry
	log      *log.Logger

	localServers map[string]localServer

	mu           sync.RWMutex
	sessions     map[string]*serverSession
	statuses     map[string]mcpconfig.ServerStatus
	refreshMu    sync.Mutex
	proxy        *mcpsdk.Server
	proxyCatalog map[string]remoteTool
	revision     uint64

	// refreshed closes when the first full refresh finishes.
	refreshed     chan struct{}
	refreshedOnce sync.Once

	proxyMu sync.Mutex

	handlerOnce sync.Once
	handler     http.Handler

	authMu     sync.Mutex
	authStates map[string]*authorizationPending

	// Optional: agents' calls to tools linked to an MCP App show it in their
	// thread (WithSessionEvents).
	eventStore sessionEventAppender
	eventBus   sessionEventPublisher
	// AppVisible, when set at startup, keeps a bot's work for other bots private.
	AppVisible func(string) bool
}

type tokenStore interface {
	integrationoauth.Store
	DeleteToken(context.Context, string) error
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
	*serverConnection
	key        [32]byte
	tools      []remoteTool
	apps       *serverApps
	searchable bool
}

type remoteTool struct {
	serverID   string
	serverName string
	remoteName string
	spec       mcpsdk.Tool
	definition tools.Definition
	connection *serverConnection
	local      bool
}

type refreshResult struct {
	server  mcpconfig.Server
	session *serverSession
	status  mcpconfig.ServerStatus
}

type connectResult struct {
	session *serverSession
	status  mcpconfig.ServerStatus
}

func NewManager(store mcpconfig.ServerReader, tokens tokenStore, registry *tools.Registry, logger *log.Logger, opts ...Option) *Manager {
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
		authStates:   make(map[string]*authorizationPending),
		proxyCatalog: make(map[string]remoteTool),
		refreshed:    make(chan struct{}),
		proxy:        mcpsdk.NewServer(&mcpsdk.Implementation{Name: ProxyServerName, Version: "0.1.0"}, &mcpsdk.ServerOptions{Capabilities: &mcpsdk.ServerCapabilities{Tools: &mcpsdk.ToolCapabilities{ListChanged: true}}}),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// backgroundHandler builds a non-interactive OAuth handler that serves stored
// tokens (refreshing them when possible) but never opens a browser. Returns nil
// when no token store is configured or the server uses a bearer token.
func (m *Manager) backgroundHandler(server mcpconfig.Server) *oauthHandler {
	if m.tokens == nil || server.UsesBearer() {
		return nil
	}
	return newOAuthHandler(server, m.tokens, http.DefaultClient)
}

func (m *Manager) Refresh(ctx context.Context) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	defer m.refreshedOnce.Do(func() { close(m.refreshed) })
	servers, err := m.servers(nil)
	if err != nil {
		return
	}
	m.refreshServerList(ctx, servers, false)
}

func (m *Manager) RefreshLocal(ctx context.Context) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	servers, err := m.servers(func(server mcpconfig.Server) bool {
		return m.hasLocalServer(server.ID)
	})
	if err != nil {
		return
	}
	m.refreshServerList(ctx, servers, true)
}

func (m *Manager) servers(include func(mcpconfig.Server) bool) ([]mcpconfig.Server, error) {
	servers, err := m.store.ListMCPServers()
	if err != nil {
		m.log.Error("load mcp servers failed", "error", err)
		return nil, err
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
	return servers, nil
}

func (m *Manager) refreshServerList(ctx context.Context, servers []mcpconfig.Server, localOnly bool) {
	m.mu.RLock()
	old := m.sessions
	m.mu.RUnlock()
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
			sessionCtx, cancel := context.WithTimeout(ctx, remoteStatusTimeout)
			defer cancel()
			key, err := m.connectionKey(sessionCtx, server)
			if err != nil {
				results[index].status = connectErrorStatus(nil, err)
				return
			}
			if previous := old[server.ID]; previous != nil && previous.key == key {
				if updated, err := previous.loadCatalog(sessionCtx, server); err == nil {
					updated.key = key
					results[index].session = updated
					results[index].status = connectedStatus(updated.tools)
					return
				}
			}
			handler := m.backgroundHandler(server)
			result, err := m.connectForStatus(sessionCtx, server, handler)
			if result.session != nil {
				result.session.key = key
			}
			results[index].status = result.status
			if err != nil {
				m.log.Warn("mcp server unavailable", "server", server.Name, "error", err)
				return
			}
			results[index].session = result.session
		}(i, server)
	}
	wg.Wait()

	next := make(map[string]*serverSession)
	statuses := make(map[string]mcpconfig.ServerStatus)
	var remoteTools, builtinTools []tools.Tool
	usedNames := map[string]string{}
	for _, result := range results {
		if result.session == nil {
			statuses[result.server.ID] = result.status
			continue
		}
		next[result.server.ID] = result.session
		builtin := m.hasLocalServer(result.server.ID)
		kept := result.session.tools[:0]
		for i := range result.session.tools {
			rt := result.session.tools[i]
			var name string
			if builtin {
				name = m.builtinToolName(result.server, rt.remoteName, usedNames)
				if name == "" {
					continue
				}
			} else {
				name = mappedToolName(result.server, rt.remoteName, usedNames)
			}
			rt.definition = tools.Function(name, rt.spec.Description, false, inputSchema(rt.spec.InputSchema))
			kept = append(kept, rt)
			if builtin {
				builtinTools = append(builtinTools, rt)
			} else {
				remoteTools = append(remoteTools, rt)
			}
		}
		result.session.tools = kept
		statuses[result.server.ID] = statusWithTools(result.status, kept)
	}

	if localOnly {
		for id, session := range old {
			if !m.hasLocalServer(id) {
				next[id] = session
				statuses[id] = m.Status(id)
				for _, tool := range session.tools {
					remoteTools = append(remoteTools, tool)
				}
			}
		}
	}
	m.mu.Lock()
	m.sessions = next
	m.statuses = statuses
	m.registry.SetGroup(BuiltinRegistryGroup, builtinTools)
	m.registry.SetGroup(RegistryGroup, remoteTools)
	m.updateProxyLocked()
	m.mu.Unlock()

	for id, previous := range old {
		if current := next[id]; current == nil || current.serverConnection != previous.serverConnection {
			closeSession(previous)
		}
	}
}

func (m *Manager) builtinToolName(server mcpconfig.Server, remote string, used map[string]string) string {
	source := server.ID + ":" + remote
	name := clampToolName(sanitizeToolName(remote), source)
	if existing, ok := used[name]; ok && existing != source {
		m.log.Warn("builtin tool name collision", "tool", name, "server", server.Name, "kept", existing)
		return ""
	}
	if m.registry.HasOutside(name, RegistryGroup, BuiltinRegistryGroup) {
		return ""
	}
	used[name] = source
	return name
}

func (m *Manager) Close() {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[string]*serverSession)
	m.updateProxyLocked()
	m.statuses = make(map[string]mcpconfig.ServerStatus)
	m.mu.Unlock()
	m.registry.RemoveGroup(BuiltinRegistryGroup)
	m.registry.RemoveGroup(RegistryGroup)
	closeSessions(sessions)
}

func closeSessions(sessions map[string]*serverSession) {
	for _, ss := range sessions {
		closeSession(ss)
	}
}

func closeSession(ss *serverSession) {
	if ss == nil {
		return
	}
	ss.retire()
}

func (m *Manager) Status(id string) mcpconfig.ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statuses[id]
}

func (m *Manager) setServerStatus(id string, status mcpconfig.ServerStatus) {
	if strings.TrimSpace(id) == "" || status.Status == "" {
		return
	}
	m.mu.Lock()
	if m.statuses == nil {
		m.statuses = make(map[string]mcpconfig.ServerStatus)
	}
	m.statuses[id] = status
	m.mu.Unlock()
}

func (m *Manager) Test(ctx context.Context, server mcpconfig.Server) mcpconfig.ServerStatus {
	if strings.TrimSpace(server.ID) == "" {
		server.ID = "test"
	}
	sessionCtx, cancel := context.WithTimeout(ctx, remoteStatusTimeout)
	defer cancel()
	handler := m.backgroundHandler(server)
	result, err := m.connectForStatus(sessionCtx, server, handler)
	if err != nil {
		return result.status
	}
	if result.session != nil {
		closeSession(result.session)
	}
	return result.status
}

// asOAuthHandler converts a possibly-nil *oauthHandler to the interface without
// producing a non-nil interface wrapping a nil pointer.
func asOAuthHandler(h *oauthHandler) auth.OAuthHandler {
	if h == nil {
		return nil
	}
	return h
}

func (m *Manager) connectForStatus(ctx context.Context, server mcpconfig.Server, handler *oauthHandler) (connectResult, error) {
	ss, err := m.connect(ctx, server, asOAuthHandler(handler))
	if err != nil {
		return connectResult{status: connectErrorStatus(handler, err)}, err
	}
	if status, ok := oauthGateStatus(ctx, server, handler, ss.tools); ok {
		closeSession(ss)
		return connectResult{status: status}, nil
	}
	return connectResult{session: ss, status: connectedStatus(ss.tools)}, nil
}

func connectedStatus(items []remoteTool) mcpconfig.ServerStatus {
	return statusWithTools(mcpconfig.ServerStatus{Status: "connected", CheckedAt: time.Now().UTC()}, items)
}

func statusWithTools(status mcpconfig.ServerStatus, items []remoteTool) mcpconfig.ServerStatus {
	status.ToolCount = len(items)
	status.Tools = serverToolViews(items)
	return status
}

func serverToolViews(items []remoteTool) []mcpconfig.ServerTool {
	if len(items) == 0 {
		return nil
	}
	out := make([]mcpconfig.ServerTool, 0, len(items))
	for _, item := range items {
		name := tools.DefinitionName(item.definition)
		if name == "" {
			name = item.remoteName
		}
		remoteName := item.remoteName
		if remoteName == name {
			remoteName = ""
		}
		out = append(out, mcpconfig.ServerTool{
			Name:        name,
			RemoteName:  remoteName,
			Description: item.spec.Description,
		})
	}
	return out
}

func connectErrorStatus(handler *oauthHandler, err error) mcpconfig.ServerStatus {
	if handler != nil && handler.needsAuthorization() {
		return mcpconfig.ServerStatus{Status: "needs_auth", Error: "Sign in required", CheckedAt: time.Now().UTC()}
	}
	return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
}

func oauthGateStatus(ctx context.Context, server mcpconfig.Server, handler *oauthHandler, items []remoteTool) (mcpconfig.ServerStatus, bool) {
	if server.UsesBearer() || (strings.TrimSpace(server.OAuth.ClientID) == "" && strings.TrimSpace(server.OAuth.Issuer) == "") {
		return mcpconfig.ServerStatus{}, false
	}
	if handler == nil {
		return mcpconfig.ServerStatus{Status: "error", Error: "token store is not configured", CheckedAt: time.Now().UTC()}, true
	}
	src, err := handler.TokenSource(ctx)
	if err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}, true
	}
	if src == nil {
		return statusWithTools(mcpconfig.ServerStatus{Status: "needs_auth", Error: "Sign in required", CheckedAt: time.Now().UTC()}, items), true
	}
	return mcpconfig.ServerStatus{}, false
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
	dial := func(ctx context.Context) (*mcpsdk.ClientSession, error) {
		return client.Connect(ctx, &mcpsdk.StreamableClientTransport{
			Endpoint:             server.URL,
			HTTPClient:           &http.Client{Transport: headerTransport{headers: headers, base: http.DefaultTransport}},
			MaxRetries:           -1,
			DisableStandaloneSSE: true,
			OAuthHandler:         handler,
		}, nil)
	}
	session, err := dial(ctx)
	if err != nil {
		return nil, err
	}
	connection := &serverConnection{session: session, redial: dial}
	ss, err := connection.loadCatalog(ctx, server)
	if err != nil {
		connection.close()
	}
	return ss, err
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
	connection := &serverConnection{session: session, localSession: localSession}
	ss, err := connection.loadCatalog(ctx, server)
	if err != nil {
		connection.close()
	}
	return ss, err
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
	res, err := t.connection.callTool(ctx, &mcpsdk.CallToolParams{
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

func (t remoteTool) callRaw(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var arguments any
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		arguments = json.RawMessage(req.Params.Arguments)
	}
	return t.connection.callTool(ctx, &mcpsdk.CallToolParams{
		Name:      t.remoteName,
		Arguments: arguments,
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
