package mcp

import (
	"context"
	"net/http"
	"reflect"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
)

func (m *Manager) Handler() http.Handler {
	m.handlerOnce.Do(func() {
		m.handler = mcpsdk.NewStreamableHTTPHandler(func(req *http.Request) *mcpsdk.Server {
			m.ensureProxyReady(req.Context())
			return m.proxy
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
