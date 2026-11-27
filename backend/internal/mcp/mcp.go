package mcp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/oauthbroker"
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
	ctx      context.Context
	cancel   context.CancelFunc

	localServers map[string]localServer

	mu           sync.RWMutex
	sessions     map[string]*serverSession
	statuses     map[string]mcpconfig.ServerStatus
	refreshGate  chan struct{}
	proxy        *mcpsdk.Server
	proxyCatalog map[string]remoteTool
	revision     uint64
	// listed is the catalog revision each agent session last read.
	listed     map[string]uint64
	sessionTTL time.Duration

	// refreshed closes when the first full refresh finishes.
	refreshed     chan struct{}
	refreshedOnce sync.Once

	proxyMu sync.Mutex

	handlerOnce sync.Once
	handler     http.Handler

	authMu     sync.Mutex
	authStates map[string]*authorizationPending
	authFlows  map[string]*authorizationPending
	broker     oauthbroker.Broker
	oauthGuard integrationoauth.RefreshGuard

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

// WithOAuthBroker configures the callback relay and hosted client metadata document.
func WithOAuthBroker(broker oauthbroker.Broker) Option {
	return func(m *Manager) {
		m.broker = broker
	}
}

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
	builtin    bool
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
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		store:        store,
		tokens:       tokens,
		registry:     registry,
		log:          logger.WithPrefix("mcp"),
		ctx:          ctx,
		cancel:       cancel,
		localServers: make(map[string]localServer),
		sessions:     make(map[string]*serverSession),
		statuses:     make(map[string]mcpconfig.ServerStatus),
		refreshGate:  make(chan struct{}, 1),
		authStates:   make(map[string]*authorizationPending),
		authFlows:    make(map[string]*authorizationPending),
		proxyCatalog: make(map[string]remoteTool),
		listed:       make(map[string]uint64),
		refreshed:    make(chan struct{}),
		sessionTTL:   30 * time.Minute,
		proxy:        mcpsdk.NewServer(&mcpsdk.Implementation{Name: ProxyServerName, Version: "0.1.0"}, &mcpsdk.ServerOptions{Capabilities: &mcpsdk.ServerCapabilities{Tools: &mcpsdk.ToolCapabilities{ListChanged: true}}}),
	}
	m.proxy.AddReceivingMiddleware(m.recordToolLists)
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// backgroundHandler builds a non-interactive OAuth handler that serves stored
// tokens (refreshing them when possible) but never opens a browser.
func (m *Manager) backgroundHandler(server mcpconfig.Server) *oauthHandler {
	if m.tokens == nil || server.UsesManualAuthorization() || server.Transport == mcpconfig.TransportStdio || server.Transport == mcpconfig.TransportSSE {
		return nil
	}
	return newOAuthHandler(server, m.tokens, http.DefaultClient, &m.oauthGuard)
}

func (m *Manager) ListMCPServers() ([]mcpconfig.Server, error) {
	return m.store.ListMCPServers()
}

func (m *Manager) Refresh(ctx context.Context) {
	m.refresh(ctx, nil)
}

func (m *Manager) RefreshLocal(ctx context.Context) {
	m.refresh(ctx, func(server mcpconfig.Server) bool {
		return m.hasLocalServer(server.ID)
	})
}

func (m *Manager) RefreshServer(ctx context.Context, id string) mcpconfig.ServerStatus {
	if err := m.refresh(ctx, func(server mcpconfig.Server) bool {
		return server.ID == id
	}); err != nil {
		return connectErrorStatus(err)
	}
	return m.Status(id)
}

func (m *Manager) RefreshServersAsync(ids ...string) {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_ = m.refresh(ctx, func(server mcpconfig.Server) bool { return selected[server.ID] })
	}()
}

func (m *Manager) refresh(ctx context.Context, include func(mcpconfig.Server) bool) error {
	select {
	case m.refreshGate <- struct{}{}:
		defer func() { <-m.refreshGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if include == nil {
		defer m.refreshedOnce.Do(func() { close(m.refreshed) })
	}
	servers, err := m.servers(nil)
	if err != nil {
		return err
	}
	m.refreshServerList(ctx, servers, include)
	return nil
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

func (m *Manager) refreshServerList(ctx context.Context, servers []mcpconfig.Server, include func(mcpconfig.Server) bool) {
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
		if include != nil && !include(server) {
			if previous := old[server.ID]; previous != nil {
				preserved := *previous
				preserved.tools = slices.Clone(previous.tools)
				results[i].session = &preserved
			}
			m.mu.RLock()
			results[i].status = m.statuses[server.ID]
			m.mu.RUnlock()
			continue
		}
		wg.Add(1)
		go func(index int, server mcpconfig.Server) {
			defer wg.Done()
			sessionCtx, cancel := context.WithTimeout(ctx, remoteStatusTimeout)
			defer cancel()
			key, err := m.connectionKey(sessionCtx, server)
			if err != nil {
				results[index].status = connectErrorStatus(err)
				return
			}
			previous := old[server.ID]
			if previous != nil && previous.key != key {
				previous = nil
			}
			result, err := m.connectForStatus(sessionCtx, server, previous)
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
			rt.builtin = builtin
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
	m.cancel()
	m.refreshGate <- struct{}{}
	defer func() { <-m.refreshGate }()
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
	return m.serverStatusLocked(id)
}

func (m *Manager) serverStatusLocked(id string) mcpconfig.ServerStatus {
	status := m.statuses[id]
	if ss := m.sessions[id]; status.Status == "connected" && ss != nil && ss.oauth != nil && ss.oauth.authorizationChallenge() != nil {
		status.Status, status.Error = "needs_auth", signInRequired
	}
	return status
}

func (m *Manager) Test(ctx context.Context, server mcpconfig.Server) mcpconfig.ServerStatus {
	if strings.TrimSpace(server.ID) == "" {
		server.ID = "test"
	}
	sessionCtx, cancel := context.WithTimeout(ctx, remoteStatusTimeout)
	defer cancel()
	result, err := m.connectForStatus(sessionCtx, server, nil)
	if err != nil {
		return result.status
	}
	if result.session != nil {
		closeSession(result.session)
	}
	return result.status
}

func (m *Manager) connectForStatus(ctx context.Context, server mcpconfig.Server, previous *serverSession) (connectResult, error) {
	var ss *serverSession
	if previous != nil {
		ss, _ = previous.loadCatalog(ctx, server)
	}
	if ss == nil {
		var err error
		ss, err = m.connect(ctx, server, m.backgroundHandler(server))
		if err != nil {
			return connectResult{status: connectErrorStatus(err)}, err
		}
	}
	if err := ss.checkOAuth(ctx, server); err != nil {
		closeSession(ss)
		return connectResult{status: statusWithTools(connectErrorStatus(err), ss.tools)}, nil
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

func connectErrorStatus(err error) mcpconfig.ServerStatus {
	if errors.Is(err, errNeedsAuthorization) || isRejectedGrant(err) {
		return mcpconfig.ServerStatus{Status: "needs_auth", Error: signInRequired, CheckedAt: time.Now().UTC()}
	}
	var refresh *integrationoauth.RefreshError
	return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC(), Retryable: !errors.As(err, &refresh)}
}

func (m *Manager) connect(ctx context.Context, server mcpconfig.Server, handler *oauthHandler) (*serverSession, error) {
	if server.LocalServer != nil {
		return m.connectLocal(ctx, server, server.LocalServer)
	}
	if local := m.localServer(server.ID); local != nil {
		return m.connectLocal(ctx, server, local)
	}
	if server.Transport == mcpconfig.TransportStdio {
		return m.connectStdio(ctx, server)
	}
	headers, err := mcpconfig.ResolvedHeaders(server, true)
	if err != nil {
		return nil, err
	}
	client := newClient()
	transport := headerTransport{headers: headers, base: http.DefaultTransport}
	if server.Source == mcpconfig.SourcePlugin {
		transport.origin, err = url.Parse(server.URL)
		if err != nil {
			return nil, err
		}
	}
	httpClient := &http.Client{Transport: transport}
	streamable := &mcpsdk.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           httpClient,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	if handler != nil {
		streamable.OAuthHandler = handler
	}
	dial := func(ctx context.Context) (*mcpsdk.ClientSession, error) {
		if server.Transport == mcpconfig.TransportSSE {
			return client.Connect(ctx, sseTransport{mcpsdk.SSEClientTransport{Endpoint: server.URL, HTTPClient: httpClient}}, nil)
		}
		return client.Connect(ctx, streamable, nil)
	}
	session, err := dial(ctx)
	if err != nil {
		return nil, err
	}
	connection := &serverConnection{session: session, redial: dial, oauth: handler}
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
	client := newClient()
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
	origin  *url.URL
	headers []mcpconfig.Header
	base    http.RoundTripper
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.origin != nil && (req.URL.Scheme != t.origin.Scheme || !strings.EqualFold(req.URL.Host, t.origin.Host)) {
		return nil, fmt.Errorf("plugin MCP request cannot change origin")
	}
	clone := req.Clone(req.Context())
	for _, header := range t.headers {
		if strings.TrimSpace(header.Name) == "" {
			continue
		}
		if t.origin == nil || clone.Header.Get(header.Name) == "" {
			clone.Header.Set(header.Name, header.Value)
		}
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
