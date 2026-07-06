package mcp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	RegistryGroup       = "mcp"
	maxToolNameLen      = 64
	remoteStatusTimeout = 25 * time.Second

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
	tokens   integrationoauth.Store
	registry *tools.Registry
	log      *log.Logger

	localServers map[string]localServer

	mu       sync.RWMutex
	sessions map[string]*serverSession
	statuses map[string]mcpconfig.ServerStatus
	refresh  uint64

	proxyMu sync.Mutex

	handlerOnce sync.Once
	handler     http.Handler

	authMu     sync.Mutex
	authStates map[string]chan loopbackResult
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
	local       bool
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
		authStates:   make(map[string]chan loopbackResult),
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
	return newOAuthHandler(server, m.tokens, http.DefaultClient)
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
			sessionCtx, cancel := context.WithTimeout(ctx, remoteStatusTimeout)
			defer cancel()
			handler := m.backgroundHandler(server)
			result, err := m.connectForStatus(sessionCtx, server, handler)
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
	var allTools []tools.Tool
	usedNames := map[string]string{}
	for _, result := range results {
		if result.session == nil {
			statuses[result.server.ID] = result.status
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
		statuses[result.server.ID] = statusWithTools(result.status, result.session.tools)
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

func (m *Manager) Handler() http.Handler {
	m.handlerOnce.Do(func() {
		m.handler = mcpsdk.NewStreamableHTTPHandler(func(req *http.Request) *mcpsdk.Server {
			m.ensureProxyReady(req.Context())
			return m.proxyServer()
		}, &mcpsdk.StreamableHTTPOptions{
			JSONResponse:   true,
			SessionTimeout: 30 * time.Minute,
		})
	})
	return m.handler
}

func (m *Manager) ensureProxyReady(ctx context.Context) {
	if !m.proxyRefreshNeeded() {
		return
	}
	m.proxyMu.Lock()
	defer m.proxyMu.Unlock()
	if !m.proxyRefreshNeeded() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, remoteStatusTimeout)
	defer cancel()
	m.Refresh(ctx)
}

func (m *Manager) proxyRefreshNeeded() bool {
	if len(m.proxyTools()) > 0 {
		return false
	}
	servers := m.remoteServers()
	if len(servers) == 0 {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, server := range servers {
		if _, ok := m.statuses[server.ID]; !ok {
			return true
		}
	}
	return false
}

func (m *Manager) remoteServers() []mcpconfig.Server {
	return m.servers(context.Background(), func(server mcpconfig.Server) bool {
		return server.Enabled && !m.hasLocalServer(server.ID)
	})
}

func (m *Manager) proxyServer() *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: ProxyServerName, Version: "0.1.0"}, nil)
	for _, tool := range m.proxyTools() {
		name := tools.DefinitionName(tool.definition)
		if name == "" {
			continue
		}
		tool := tool
		server.AddTool(&mcpsdk.Tool{
			Name:        name,
			Description: tool.description,
			InputSchema: tool.inputSchema,
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return tool.callRaw(ctx, req)
		})
	}
	return server
}

func (m *Manager) proxyTools() []remoteTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []remoteTool
	for _, session := range m.sessions {
		for _, tool := range session.tools {
			if tool.local {
				continue
			}
			out = append(out, tool)
		}
	}
	return out
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
	_ = ss.session.Close()
	if ss.localSession != nil {
		_ = ss.localSession.Close()
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

// Authorize runs the interactive OAuth authorization-code flow for a server,
// persists the resulting token, and reconnects so the server's tools become
// available. Browser clients receive the provider URL immediately and finish
// through the shared callback route; desktop callers keep the historical blocking
// loopback flow.
func (m *Manager) Authorize(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions) mcpconfig.ServerStatus {
	if m.tokens == nil {
		return mcpconfig.ServerStatus{Status: "error", Error: "token store is not configured", CheckedAt: time.Now().UTC()}
	}
	if opts.ReturnAuthURL {
		return m.authorizeWithCallback(ctx, server, opts)
	}
	receiver, err := newLoopbackReceiver()
	if err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}
	defer receiver.close()

	return m.runAuthorize(ctx, server, receiver.redirectURL, receiver.fetch)
}

func (m *Manager) authorizeWithCallback(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions) mcpconfig.ServerStatus {
	if strings.TrimSpace(opts.RedirectURL) == "" {
		return mcpconfig.ServerStatus{Status: "error", Error: "OAuth redirect URL is not configured", CheckedAt: time.Now().UTC()}
	}
	started := make(chan authorizationStart, 1)
	done := make(chan mcpconfig.ServerStatus, 1)
	receiver := &callbackReceiver{
		manager:     m,
		redirectURL: strings.TrimSpace(opts.RedirectURL),
		started:     started,
		openBrowser: opts.OpenBrowser,
	}
	flowCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	go func() {
		defer cancel()
		done <- m.runAuthorize(flowCtx, server, receiver.redirectURL, receiver.fetch)
	}()

	select {
	case start := <-started:
		return mcpconfig.ServerStatus{
			Status:    "needs_auth",
			Error:     "Sign in required",
			AuthURL:   start.authURL,
			CheckedAt: time.Now().UTC(),
		}
	case status := <-done:
		return status
	case <-ctx.Done():
		return mcpconfig.ServerStatus{Status: "error", Error: ctx.Err().Error(), CheckedAt: time.Now().UTC()}
	}
}

func (m *Manager) runAuthorize(ctx context.Context, server mcpconfig.Server, redirectURL string, fetch codeFetcher) mcpconfig.ServerStatus {
	handler := newOAuthHandler(server, m.tokens, http.DefaultClient)
	handler.mode = oauthModeInteractive
	handler.redirectURL = redirectURL
	handler.fetch = fetch

	sessionCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	ss, err := m.connect(sessionCtx, server, handler)
	if err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}
	closeSessions(map[string]*serverSession{server.ID: ss})
	if !handler.didAuthorize() {
		if err := handler.AuthorizeFromMetadata(sessionCtx); err != nil {
			return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
		}
	}

	// Reconnect everything so this server's tools are installed in the registry
	// using the token we just persisted.
	m.Refresh(context.Background())
	if status := m.Status(server.ID); status.Status != "" {
		return status
	}
	return connectedStatus(ss.tools)
}

type authorizationStart struct {
	authURL string
}

type callbackReceiver struct {
	manager     *Manager
	redirectURL string
	started     chan<- authorizationStart
	openBrowser bool
}

func (r *callbackReceiver) fetch(ctx context.Context, authURL string) (string, string, error) {
	state := authorizationStateFromURL(authURL)
	if state == "" {
		return "", "", errors.New("authorization URL did not include state")
	}
	result := make(chan loopbackResult, 1)
	if err := r.manager.registerAuthorizationState(state, result); err != nil {
		return "", "", err
	}
	defer r.manager.unregisterAuthorizationState(state, result)

	if r.openBrowser {
		if err := openBrowser(authURL); err != nil {
			fmt.Printf("Open this URL to authorize the MCP server:\n%s\n", authURL)
		}
	}
	select {
	case r.started <- authorizationStart{authURL: authURL}:
	default:
	}
	select {
	case res := <-result:
		if res.err != "" {
			return "", "", fmt.Errorf("authorization failed: %s", res.err)
		}
		if res.code == "" {
			return "", "", errors.New("authorization returned no code")
		}
		return res.code, res.state, nil
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

func (m *Manager) CompleteAuthorization(ctx context.Context, state, code, failure string) error {
	state = strings.TrimSpace(state)
	if state == "" {
		return errors.New("authorization state is required")
	}
	result, ok := m.takeAuthorizationState(state)
	if !ok {
		return errors.New("authorization state expired or was not started by Jaz")
	}
	select {
	case result <- loopbackResult{code: strings.TrimSpace(code), state: state, err: strings.TrimSpace(failure)}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) registerAuthorizationState(state string, result chan loopbackResult) error {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	if m.authStates == nil {
		m.authStates = make(map[string]chan loopbackResult)
	}
	if _, exists := m.authStates[state]; exists {
		return errors.New("authorization state is already pending")
	}
	m.authStates[state] = result
	return nil
}

func (m *Manager) unregisterAuthorizationState(state string, result chan loopbackResult) {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	if m.authStates[state] == result {
		delete(m.authStates, state)
	}
}

func (m *Manager) takeAuthorizationState(state string) (chan loopbackResult, bool) {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	result, ok := m.authStates[state]
	if ok {
		delete(m.authStates, state)
	}
	return result, ok
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
			Description: item.description,
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
	if strings.TrimSpace(server.OAuth.ClientID) == "" && strings.TrimSpace(server.OAuth.Issuer) == "" {
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
			local:       true,
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

func (t remoteTool) callRaw(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var arguments any
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		arguments = json.RawMessage(req.Params.Arguments)
	}
	return t.session.CallTool(ctx, &mcpsdk.CallToolParams{
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
