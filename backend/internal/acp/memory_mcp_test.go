package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

func TestMCPServersForAgentUsesConfiguredStore(t *testing.T) {
	m := &Manager{cfg: Config{MCPStore: staticMCPServerStore{servers: []mcpconfig.Server{{
		ID:        "jaztools",
		Name:      "jaztools",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       "http://127.0.0.1:5299/mcp/jaztools",
		Enabled:   true,
		Headers: []mcpconfig.Header{{
			Name:  mcpsession.HeaderName,
			Value: mcpsession.HeaderPlaceholder,
		}},
	}}}}}
	capable := json.RawMessage(`{"agentCapabilities":{"mcpCapabilities":{"http":true}}}`)
	servers := m.mcpServersForAgent(context.Background(), capable)
	if len(servers) != 1 {
		t.Fatalf("expected configured jaz entry, got %v", servers)
	}
	var entry struct {
		Type    string             `json:"type"`
		Name    string             `json:"name"`
		URL     string             `json:"url"`
		Headers []mcpconfig.Header `json:"headers"`
	}
	if err := json.Unmarshal(servers[0], &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Type != "http" || entry.Name != "jaztools" || entry.URL != "http://127.0.0.1:5299/mcp/jaztools" {
		t.Fatalf("unexpected entry %#v", entry)
	}
	if got := headerValue(entry.Headers, mcpsession.HeaderName); got != "" {
		t.Fatalf("unexpected session header without context: %q", got)
	}
	servers = m.mcpServersForAgent(mcpsession.With(context.Background(), "thread-1"), capable)
	if len(servers) != 1 {
		t.Fatalf("expected configured jaz entry with session context, got %v", servers)
	}
	if err := json.Unmarshal(servers[0], &entry); err != nil {
		t.Fatal(err)
	}
	if got := headerValue(entry.Headers, mcpsession.HeaderName); got != "thread-1" {
		t.Fatalf("session header = %q", got)
	}

	incapable := json.RawMessage(`{"agentCapabilities":{}}`)
	if servers := m.mcpServersForAgent(context.Background(), incapable); len(servers) != 0 {
		t.Fatalf("non-http-capable agent must get nothing, got %v", servers)
	}
}

func headerValue(headers []mcpconfig.Header, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}
