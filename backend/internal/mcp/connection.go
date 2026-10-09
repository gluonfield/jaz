package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type serverConnection struct {
	session      *mcpsdk.ClientSession
	localSession *mcpsdk.ServerSession
	// redial opens a fresh session to the same server; nil for local servers,
	// whose sessions live in process.
	redial   func(context.Context) (*mcpsdk.ClientSession, error)
	resumeMu sync.Mutex
	mu       sync.Mutex
	calls    int
	retired  bool
}

// acquire holds the connection open for one request; a retired connection
// closes once its last in-flight request releases it.
func (c *serverConnection) acquire() (func(), error) {
	c.mu.Lock()
	if c.retired {
		c.mu.Unlock()
		return nil, errors.New("MCP server configuration changed")
	}
	c.calls++
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		c.calls--
		closeNow := c.retired && c.calls == 0
		c.mu.Unlock()
		if closeNow {
			c.close()
		}
	}, nil
}

func (c *serverConnection) callTool(ctx context.Context, params *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	return use(ctx, c, func(session *mcpsdk.ClientSession) (*mcpsdk.CallToolResult, error) {
		return session.CallTool(ctx, params)
	})
}

func (c *serverConnection) readResource(ctx context.Context, uri string) (*mcpsdk.ReadResourceResult, error) {
	return use(ctx, c, func(session *mcpsdk.ClientSession) (*mcpsdk.ReadResourceResult, error) {
		return session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: uri})
	})
}

// use runs a request on the live session. When the server has dropped the
// session, as it does on restart, it rejected the request unseen, so the
// connection opens a new session and sends it once more.
func use[T any](ctx context.Context, c *serverConnection, request func(*mcpsdk.ClientSession) (T, error)) (T, error) {
	var none T
	release, err := c.acquire()
	if err != nil {
		return none, err
	}
	defer release()
	session := c.current()
	result, err := request(session)
	if c.redial == nil || (!errors.Is(err, mcpsdk.ErrSessionMissing) && !errors.Is(err, mcpsdk.ErrConnectionClosed)) {
		return result, err
	}
	if session, err = c.resume(ctx, session); err != nil {
		return none, err
	}
	return request(session)
}

func (c *serverConnection) current() *mcpsdk.ClientSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session
}

// resume replaces a lost session unless a concurrent request already did.
func (c *serverConnection) resume(ctx context.Context, lost *mcpsdk.ClientSession) (*mcpsdk.ClientSession, error) {
	c.resumeMu.Lock()
	defer c.resumeMu.Unlock()
	if session := c.current(); session != lost {
		return session, nil
	}
	session, err := c.redial(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.session = session
	c.mu.Unlock()
	_ = lost.Close()
	return session, nil
}

func (c *serverConnection) retire() {
	c.mu.Lock()
	closeNow := !c.retired && c.calls == 0
	c.retired = true
	c.mu.Unlock()
	if closeNow {
		c.close()
	}
}

func (c *serverConnection) close() {
	_ = c.current().Close()
	if c.localSession != nil {
		_ = c.localSession.Close()
	}
}

func (m *Manager) connectionKey(ctx context.Context, server mcpconfig.Server) ([32]byte, error) {
	headers, err := mcpconfig.ResolvedHeaders(server, true)
	if err != nil {
		return [32]byte{}, err
	}
	var token integrationoauth.Token
	if m.tokens != nil && !m.hasLocalServer(server.ID) {
		token, _, err = m.tokens.LoadToken(ctx, server.TokenID())
		if err != nil {
			return [32]byte{}, err
		}
	}
	data, err := json.Marshal(struct {
		URL     string
		Headers []mcpconfig.Header
		OAuth   mcpconfig.OAuthConfig
		Token   integrationoauth.Token
		TokenID string
	}{server.URL, headers, server.OAuth, token, server.TokenID()})
	return sha256.Sum256(data), err
}

// loadCatalog reads the server's agent tools, what its tools declare for MCP
// Apps and whether it offers search.
func (c *serverConnection) loadCatalog(ctx context.Context, server mcpconfig.Server) (*serverSession, error) {
	var items []remoteTool
	session := c.current()
	apps := newServerApps(server.ID, session.InitializeResult())
	searches := false
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		if tool == nil || tool.Name == "" {
			continue
		}
		searches = searches || isSearchTool(tool)
		meta := readToolMeta(tool)
		apps.add(tool, meta)
		if !meta.visibleTo("model") {
			continue
		}
		spec := *tool
		spec.Description = toolDescription(server, tool)
		items = append(items, remoteTool{
			serverID:   server.ID,
			serverName: server.Name,
			remoteName: tool.Name,
			connection: c,
			local:      c.localSession != nil,
			spec:       spec,
		})
	}
	return &serverSession{serverConnection: c, tools: items, apps: apps, searchable: searches}, nil
}
