package acp

import (
	"encoding/json"
	"testing"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

func TestMCPServersForAgentUsesConfiguredStore(t *testing.T) {
	m := &Manager{cfg: Config{MCPStore: staticMCPServerStore{servers: []mcpconfig.Server{{
		ID:        "jazmem",
		Name:      "jazmem",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       "http://127.0.0.1:5299/mcp/jazmem",
		Enabled:   true,
	}}}}}
	capable := json.RawMessage(`{"agentCapabilities":{"mcpCapabilities":{"http":true}}}`)
	servers := m.mcpServersForAgent(capable)
	if len(servers) != 1 {
		t.Fatalf("expected configured jazmem entry, got %v", servers)
	}
	var entry struct {
		Type string `json:"type"`
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(servers[0], &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Type != "http" || entry.Name != "jazmem" || entry.URL != "http://127.0.0.1:5299/mcp/jazmem" {
		t.Fatalf("unexpected entry %#v", entry)
	}

	incapable := json.RawMessage(`{"agentCapabilities":{}}`)
	if servers := m.mcpServersForAgent(incapable); len(servers) != 0 {
		t.Fatalf("non-http-capable agent must get nothing, got %v", servers)
	}
}
