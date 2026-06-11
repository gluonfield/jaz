package acp

import (
	"encoding/json"

	acpschema "github.com/gluonfield/acp-transport/acp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

type MemoryMCP struct {
	URL     string
	Enabled func() bool
}

func (m *Manager) mcpServersForAgent(initRaw json.RawMessage) []json.RawMessage {
	var init struct {
		AgentCapabilities acpschema.AgentCapabilities `json:"agentCapabilities"`
	}
	if err := json.Unmarshal(initRaw, &init); err != nil {
		return []json.RawMessage{}
	}
	if init.AgentCapabilities.MCPCapabilities == nil || !init.AgentCapabilities.MCPCapabilities.HTTP {
		return []json.RawMessage{}
	}
	servers, err := enabledHTTPMCPServers(m.cfg.MCPStore)
	if err != nil {
		m.log.Warn("load mcp servers for acp failed", "error", err)
		servers = nil
	}
	if entry := m.memoryMCPServer(); entry != nil {
		servers = append(servers, entry)
	}
	if servers == nil {
		return []json.RawMessage{}
	}
	return servers
}

func (m *Manager) memoryMCPServer() json.RawMessage {
	if m.MemoryMCP.URL == "" {
		return nil
	}
	if m.MemoryMCP.Enabled != nil && !m.MemoryMCP.Enabled() {
		return nil
	}
	payload := struct {
		Type    string             `json:"type"`
		Name    string             `json:"name"`
		URL     string             `json:"url"`
		Headers []mcpconfig.Header `json:"headers"`
	}{
		Type:    "http",
		Name:    "jazmem",
		URL:     m.MemoryMCP.URL,
		Headers: []mcpconfig.Header{},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		m.log.Warn("marshal memory mcp server failed", "error", err)
		return nil
	}
	return json.RawMessage(data)
}

func enabledHTTPMCPServers(store mcpconfig.ServerReader) ([]json.RawMessage, error) {
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
		headers, err := mcpconfig.ResolvedHeaders(server, false)
		if err != nil {
			return nil, err
		}
		payload := struct {
			Type    string             `json:"type"`
			Name    string             `json:"name"`
			URL     string             `json:"url"`
			Headers []mcpconfig.Header `json:"headers"`
		}{
			Type:    "http",
			Name:    server.Name,
			URL:     server.URL,
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
