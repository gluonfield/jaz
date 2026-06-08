package acp

import (
	"encoding/json"
	"testing"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

type staticMCPServerStore struct {
	servers []mcpconfig.Server
}

func (s staticMCPServerStore) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), s.servers...), nil
}

func TestEnabledHTTPMCPServersEmitsRawHTTPPayloads(t *testing.T) {
	t.Setenv("MCP_SECRET", "secret")
	servers, err := enabledHTTPMCPServers(staticMCPServerStore{servers: []mcpconfig.Server{
		{
			Name:       "Docs",
			URL:        "https://mcp.example.com/mcp",
			Enabled:    true,
			Transport:  mcpconfig.TransportStreamableHTTP,
			Headers:    []mcpconfig.Header{{Name: "X-Literal", Value: "literal"}},
			EnvHeaders: []mcpconfig.EnvHeader{{Name: "X-Secret", EnvVar: "MCP_SECRET"}},
		},
		{
			Name:      "Disabled",
			URL:       "https://disabled.example.com/mcp",
			Enabled:   false,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(servers))
	}
	var payload struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
	}
	if err := json.Unmarshal(servers[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Type != "http" || payload.Name != "Docs" || payload.URL != "https://mcp.example.com/mcp" {
		t.Fatalf("payload = %#v", payload)
	}
	headers := map[string]string{}
	for _, header := range payload.Headers {
		headers[header.Name] = header.Value
	}
	if headers["X-Literal"] != "literal" || headers["X-Secret"] != "secret" {
		t.Fatalf("headers = %#v", headers)
	}
}
