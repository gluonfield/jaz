package acp

import (
	"encoding/json"
	"testing"
)

func TestMemoryMCPInjectedForCapableAgents(t *testing.T) {
	m := &Manager{}
	capable := json.RawMessage(`{"agentCapabilities":{"mcpCapabilities":{"http":true}}}`)

	if servers := m.mcpServersForAgent(capable); len(servers) != 0 {
		t.Fatalf("no memory mcp configured, expected none, got %v", servers)
	}

	m.MemoryMCP = MemoryMCP{URL: "http://127.0.0.1:5299/mcp/jazmem"}
	servers := m.mcpServersForAgent(capable)
	if len(servers) != 1 {
		t.Fatalf("expected synthetic jazmem entry, got %v", servers)
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

	m.MemoryMCP.Enabled = func() bool { return false }
	if servers := m.mcpServersForAgent(capable); len(servers) != 0 {
		t.Fatalf("disabled memory must not be injected, got %v", servers)
	}

	m.MemoryMCP.Enabled = nil
	incapable := json.RawMessage(`{"agentCapabilities":{}}`)
	if servers := m.mcpServersForAgent(incapable); len(servers) != 0 {
		t.Fatalf("non-http-capable agent must get nothing, got %v", servers)
	}
}
