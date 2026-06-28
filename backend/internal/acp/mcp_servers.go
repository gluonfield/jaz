package acp

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

const jaztoolsSurfaceQueryParam = "jaztools_surface"

func (m *Manager) mcpServersForAgent(ctx context.Context, initRaw json.RawMessage, policy string) []json.RawMessage {
	var init struct {
		AgentCapabilities acpschema.AgentCapabilities `json:"agentCapabilities"`
	}
	if err := json.Unmarshal(initRaw, &init); err != nil {
		return []json.RawMessage{}
	}
	if init.AgentCapabilities.MCPCapabilities == nil || !init.AgentCapabilities.MCPCapabilities.HTTP {
		return []json.RawMessage{}
	}
	servers, err := enabledHTTPMCPServers(ctx, m.cfg.MCPStore, policy)
	if err != nil {
		m.log.Warn("load mcp servers for acp failed", "error", err)
		servers = nil
	}
	if servers == nil {
		return []json.RawMessage{}
	}
	return servers
}

func enabledHTTPMCPServers(ctx context.Context, store mcpconfig.ServerReader, policy string) ([]json.RawMessage, error) {
	if store == nil {
		return nil, nil
	}
	servers, err := store.ListMCPServers()
	if err != nil {
		return nil, err
	}
	var out []json.RawMessage
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		if server.Transport != mcpconfig.TransportStreamableHTTP {
			continue
		}
		if !mcpServerAllowed(policy, server) {
			continue
		}
		headers := resolvedACPHeaders(ctx, server)
		if headers == nil {
			// codex-acp's schema requires an array; null fails session/new.
			headers = []mcpconfig.Header{}
		}
		payload := struct {
			Type    string             `json:"type"`
			Name    string             `json:"name"`
			URL     string             `json:"url"`
			Headers []mcpconfig.Header `json:"headers"`
		}{
			Type:    "http",
			Name:    server.Name,
			URL:     mcpServerURL(policy, server),
			Headers: headers,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(data))
	}
	return out, nil
}

func mcpServerAllowed(policy string, server mcpconfig.Server) bool {
	policy = strings.TrimSpace(policy)
	if restrictedWorkerPolicy(policy) {
		return isJaztoolsServer(server)
	}
	switch policy {
	case MCPServerPolicyAll, MCPServerPolicyWidget:
		return true
	default:
		return false
	}
}

// mcpServerURL routes the jaztools server to the tool surface named by the
// policy itself: the policy string and the jaztools surface query param are one
// and the same, so they cannot drift out of sync.
func mcpServerURL(policy string, server mcpconfig.Server) string {
	policy = strings.TrimSpace(policy)
	if !isJaztoolsServer(server) || policy == MCPServerPolicyAll {
		return server.URL
	}
	return jaztoolsSurfaceURL(server.URL, policy)
}

func isJaztoolsServer(server mcpconfig.Server) bool {
	return strings.EqualFold(strings.TrimSpace(server.ID), "jaztools") ||
		strings.EqualFold(strings.TrimSpace(server.Name), "jaztools")
}

func jaztoolsSurfaceURL(raw, surface string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set(jaztoolsSurfaceQueryParam, surface)
	u.RawQuery = q.Encode()
	return u.String()
}

func resolvedACPHeaders(ctx context.Context, server mcpconfig.Server) []mcpconfig.Header {
	return resolveSessionHeaders(ctx, server.Headers)
}

func resolveSessionHeaders(ctx context.Context, headers []mcpconfig.Header) []mcpconfig.Header {
	sessionID := mcpsession.ID(ctx)
	out := make([]mcpconfig.Header, 0, len(headers))
	for _, header := range headers {
		header.Name = strings.TrimSpace(header.Name)
		if !strings.EqualFold(header.Name, mcpsession.HeaderName) {
			continue
		}
		if header.Value != mcpsession.HeaderPlaceholder {
			if strings.TrimSpace(header.Value) != "" {
				out = append(out, header)
			}
			continue
		}
		if sessionID != "" {
			header.Value = sessionID
			out = append(out, header)
		}
	}
	return out
}
