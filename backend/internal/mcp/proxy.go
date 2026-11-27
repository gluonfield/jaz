package mcp

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpdeferred"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/tools"
)

func (m *Manager) Handler() http.Handler {
	m.handlerOnce.Do(func() {
		m.handler = mcpdeferred.Wrap(keepStreamingSessions(mcpsdk.NewStreamableHTTPHandler(func(req *http.Request) *mcpsdk.Server {
			m.ensureProxyReady(req.Context())
			return m.proxy
		}, &mcpsdk.StreamableHTTPOptions{
			JSONResponse:   true,
			SessionTimeout: m.sessionTTL,
		}), m.sessionTTL/3))
	})
	return m.handler
}

type discardResponse struct{ header http.Header }

func (d discardResponse) Header() http.Header       { return d.header }
func (discardResponse) WriteHeader(int)             {}
func (discardResponse) Write(b []byte) (int, error) { return len(b), nil }

// The SDK expires a session that sends no request, even one whose agent still
// holds its notification stream, and an expired session never hears
// tools/list_changed. Pinging on the stream's behalf ties the session to it.
func keepStreamingSessions(next http.Handler, interval time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			ping := r.Clone(r.Context())
			ping.Method = http.MethodPost
			ping.Header.Set("Content-Type", "application/json")
			ping.Header.Set("Accept", "application/json, text/event-stream")
			go func() {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ping.Context().Done():
						return
					case <-ticker.C:
						ping.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"jaz-keepalive","method":"ping"}`))
						next.ServeHTTP(discardResponse{make(http.Header)}, ping)
					}
				}
			}()
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Manager) recordToolLists(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
	return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
		if extra := req.GetExtra(); method == "tools/list" && extra != nil {
			m.mu.Lock()
			m.listed[extra.Header.Get(mcpsession.HeaderName)] = m.revision
			m.mu.Unlock()
		}
		return next(ctx, method, req)
	}
}

// ListedRevision is the catalog revision the agent session's MCP client last read.
func (m *Manager) ListedRevision(session string) uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listed[session]
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
	_ = m.refresh(ctx, func(server mcpconfig.Server) bool {
		return m.Status(server.ID).Status == ""
	})
}

func (m *Manager) proxyRefreshNeeded() bool {
	m.mu.RLock()
	ready := len(m.proxyCatalog) > 0
	m.mu.RUnlock()
	if ready {
		return false
	}
	servers, err := m.servers(func(server mcpconfig.Server) bool {
		return server.Enabled && !m.hasLocalServer(server.ID)
	})
	if err != nil {
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

func (m *Manager) Revision() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.revision
}

func (m *Manager) updateProxyLocked() {
	next := make(map[string]remoteTool)
	changed := false
	for _, session := range m.sessions {
		for _, tool := range session.tools {
			if tool.builtin {
				continue
			}
			name := tools.DefinitionName(tool.definition)
			next[name] = tool
			previous, exists := m.proxyCatalog[name]
			if exists && previous.connection == tool.connection && reflect.DeepEqual(previous.spec, tool.spec) {
				continue
			}
			changed = true
			spec := tool.spec
			spec.Name = name
			m.proxy.AddTool(&spec, m.proxyCall(tool))
		}
	}
	for name := range m.proxyCatalog {
		if _, exists := next[name]; !exists {
			changed = true
			m.proxy.RemoveTools(name)
		}
	}
	m.proxyCatalog = next
	if changed {
		m.revision++
	}
}
