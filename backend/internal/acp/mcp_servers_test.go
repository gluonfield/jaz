package acp

import (
	"context"
	"encoding/json"
	"testing"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

type staticMCPServerStore struct {
	servers []mcpconfig.Server
}

func (s staticMCPServerStore) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), s.servers...), nil
}

func TestEnabledHTTPMCPServersEmitsConfiguredHTTPPayloads(t *testing.T) {
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
		{
			Name:      "jaz_mcp",
			URL:       "http://127.0.0.1:5299/mcp/proxy",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
		{
			Name:      "Disabled",
			URL:       "https://disabled.example.com/mcp",
			Enabled:   false,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
	}}, MCPServerPolicyAll)
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
	if payload.Type != "http" || payload.Name != "jaz_mcp" || payload.URL != "http://127.0.0.1:5299/mcp/proxy" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload.Headers) != 0 {
		t.Fatalf("headers = %#v, want none", payload.Headers)
	}
}

func TestEnabledHTTPMCPServersAppliesPolicyBeforeResolvingHeaders(t *testing.T) {
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
		{
			ID:        "docs",
			Name:      "Docs",
			URL:       "https://docs.example.com/mcp",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
		{
			ID:        "jaztools",
			Name:      "jaztools",
			URL:       "http://127.0.0.1:5299/mcp/jaztools",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
	}}, MCPServerPolicyMemorySearchWorker)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want only jaztools", len(servers))
	}
	var payload struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(servers[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Name != "jaztools" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.URL != "http://127.0.0.1:5299/mcp/jaztools?jaztools_surface=memory_search_worker" {
		t.Fatalf("url = %q", payload.URL)
	}
}

func TestEnabledHTTPMCPServersJaztoolsOnlyPolicyLeavesURLUnchanged(t *testing.T) {
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{{
		ID:        "jaztools",
		Name:      "jaztools",
		URL:       "http://127.0.0.1:5299/mcp/jaztools",
		Enabled:   true,
		Transport: mcpconfig.TransportStreamableHTTP,
	}}}, MCPServerPolicyJaztoolsOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want jaztools", len(servers))
	}
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(servers[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.URL != "http://127.0.0.1:5299/mcp/jaztools" {
		t.Fatalf("url = %q", payload.URL)
	}
}

func TestEnabledHTTPMCPServersWidgetPolicyAddsSurface(t *testing.T) {
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
		{
			ID:        "docs",
			Name:      "Docs",
			URL:       "https://docs.example.com/mcp",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
		{
			ID:        "jaztools",
			Name:      "jaztools",
			URL:       "http://127.0.0.1:5299/mcp/jaztools",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
	}}, MCPServerPolicyWidget)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want only jaztools", len(servers))
	}
	var payload struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(servers[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Name != "jaztools" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.URL != "http://127.0.0.1:5299/mcp/jaztools?jaztools_surface=widget" {
		t.Fatalf("url = %q", payload.URL)
	}
}

func TestEnabledHTTPMCPServersUnknownPolicyFailsClosed(t *testing.T) {
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{{
		ID:        "docs",
		Name:      "Docs",
		URL:       "https://docs.example.com/mcp",
		Enabled:   true,
		Transport: mcpconfig.TransportStreamableHTTP,
	}}}, "unknown-policy")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers = %d, want none for unknown policy", len(servers))
	}
}

func TestEnabledHTTPMCPServersOnlyCarriesSessionHeader(t *testing.T) {
	ctx := mcpsession.With(context.Background(), "session-1")
	servers, err := enabledHTTPMCPServers(ctx, staticMCPServerStore{servers: []mcpconfig.Server{
		{
			ID:                "jaztools",
			Name:              "jaztools",
			URL:               "http://127.0.0.1:5299/mcp/jaztools",
			Enabled:           true,
			Transport:         mcpconfig.TransportStreamableHTTP,
			BearerTokenEnvVar: "MCP_TOKEN",
			Headers: []mcpconfig.Header{
				{Name: "Authorization", Value: "Bearer configured"},
				{Name: "X-API-Key", Value: "configured"},
				{Name: mcpsession.HeaderName, Value: mcpsession.HeaderPlaceholder},
			},
			EnvHeaders: []mcpconfig.EnvHeader{{Name: "X-Env-Secret", EnvVar: "MCP_SECRET"}},
		},
	}}, MCPServerPolicyAll)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Headers []mcpconfig.Header `json:"headers"`
	}
	if err := json.Unmarshal(servers[0], &payload); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{}
	for _, header := range payload.Headers {
		headers[header.Name] = header.Value
	}
	if len(headers) != 1 || headers[mcpsession.HeaderName] != "session-1" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestEnabledHTTPMCPServersEmitsEmptyHeadersArray(t *testing.T) {
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
		{
			Name:      "Memory",
			URL:       "http://127.0.0.1:5299/mcp/jazmem",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
	}}, MCPServerPolicyAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(servers))
	}
	var payload struct {
		Type    string             `json:"type"`
		Name    string             `json:"name"`
		URL     string             `json:"url"`
		Headers []mcpconfig.Header `json:"headers"`
	}
	if err := json.Unmarshal(servers[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Type != "http" || payload.Name != "Memory" || payload.URL != "http://127.0.0.1:5299/mcp/jazmem" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Headers == nil || len(payload.Headers) != 0 {
		t.Fatalf("headers = %#v, want empty array", payload.Headers)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(servers[0], &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["headers"]) != "[]" {
		t.Fatalf("raw headers = %s, want []", raw["headers"])
	}
}
