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
	servers := m.mcpServersForAgent(context.Background(), capable, MCPServerPolicyAll)
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
	servers = m.mcpServersForAgent(mcpsession.With(context.Background(), "thread-1"), capable, MCPServerPolicyAll)
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
	if servers := m.mcpServersForAgent(context.Background(), incapable, MCPServerPolicyAll); len(servers) != 0 {
		t.Fatalf("non-http-capable agent must get nothing, got %v", servers)
	}
}

func TestMCPServersForJaztoolsOnlyPolicyOnlyExposesJaztools(t *testing.T) {
	m := &Manager{
		cfg: Config{MCPStore: staticMCPServerStore{servers: []mcpconfig.Server{
			{
				ID:        "docs",
				Name:      "Docs",
				Transport: mcpconfig.TransportStreamableHTTP,
				URL:       "https://docs.example.com/mcp",
				Enabled:   true,
			},
			{
				ID:        "jaztools",
				Name:      "jaztools",
				Transport: mcpconfig.TransportStreamableHTTP,
				URL:       "http://127.0.0.1:5299/mcp/jaztools",
				Enabled:   true,
				Headers: []mcpconfig.Header{{
					Name:  mcpsession.HeaderName,
					Value: mcpsession.HeaderPlaceholder,
				}},
			},
		}}},
	}
	capable := json.RawMessage(`{"agentCapabilities":{"mcpCapabilities":{"http":true}}}`)
	servers := m.mcpServersForAgent(mcpsession.With(context.Background(), "search-session"), capable, MCPServerPolicyMemorySearchWorker)
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want only jaztools", len(servers))
	}
	var entry struct {
		Name    string             `json:"name"`
		URL     string             `json:"url"`
		Headers []mcpconfig.Header `json:"headers"`
	}
	if err := json.Unmarshal(servers[0], &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Name != "jaztools" {
		t.Fatalf("entry = %#v, want jaztools", entry)
	}
	if !strings.Contains(entry.URL, "jaztools_surface=memory_search_worker") {
		t.Fatalf("entry url = %q, want memory search surface", entry.URL)
	}
	if got := headerValue(entry.Headers, mcpsession.HeaderName); got != "search-session" {
		t.Fatalf("session header = %q, want search-session", got)
	}

	servers = m.mcpServersForAgent(mcpsession.With(context.Background(), "browser-session"), capable, MCPServerPolicyBrowserWorker)
	if len(servers) != 1 {
		t.Fatalf("browser servers = %d, want only jaztools", len(servers))
	}
	if err := json.Unmarshal(servers[0], &entry); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entry.URL, "jaztools_surface=browser_worker") {
		t.Fatalf("entry url = %q, want browser worker surface", entry.URL)
	}
	if got := headerValue(entry.Headers, mcpsession.HeaderName); got != "browser-session" {
		t.Fatalf("session header = %q, want browser-session", got)
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
