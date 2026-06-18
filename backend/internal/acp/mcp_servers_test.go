package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type staticMCPServerStore struct {
	servers []mcpconfig.Server
}

func (s staticMCPServerStore) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), s.servers...), nil
}

type staticMCPTokens map[string]integrationoauth.Token

func (s staticMCPTokens) LoadToken(_ context.Context, id string) (integrationoauth.Token, bool, error) {
	token, ok := s[id]
	return token, ok, nil
}

func (s staticMCPTokens) SaveToken(_ context.Context, id string, token integrationoauth.Token) error {
	s[id] = token
	return nil
}

func TestEnabledHTTPMCPServersEmitsRawHTTPPayloads(t *testing.T) {
	t.Setenv("MCP_SECRET", "secret")
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
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
	}}, nil, MCPServerPolicyAll)
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

func TestEnabledHTTPMCPServersAddsStoredOAuthToken(t *testing.T) {
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
		{
			ID:        "n8n",
			Name:      "n8n",
			URL:       "https://mcp.example.com/mcp",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
	}}, staticMCPTokens{mcpconfig.OAuthConnectionID("n8n"): {AccessToken: "oauth-token"}}, MCPServerPolicyAll)
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
	if headers["Authorization"] != "Bearer oauth-token" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestEnabledHTTPMCPServersRefreshesStoredOAuthToken(t *testing.T) {
	tokenEndpointCalled := false
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenEndpointCalled = true
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Fatalf("unexpected refresh request %s", r.Form.Encode())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "fresh-token",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	tokenID := mcpconfig.OAuthConnectionID("n8n")
	tokens := staticMCPTokens{tokenID: {
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
		ClientID:     "client",
		TokenURL:     tokenServer.URL,
		AuthStyle:    int(oauth2.AuthStyleInParams),
	}}
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
		{
			ID:        "n8n",
			Name:      "n8n",
			URL:       "https://mcp.example.com/mcp",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
	}}, tokens, MCPServerPolicyAll)
	if err != nil {
		t.Fatal(err)
	}
	if !tokenEndpointCalled {
		t.Fatal("expected token refresh")
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
	if headers["Authorization"] != "Bearer fresh-token" || tokens[tokenID].AccessToken != "fresh-token" {
		t.Fatalf("headers = %#v token = %#v", headers, tokens[tokenID])
	}
}

func TestEnabledHTTPMCPServersSkipsOnlyServerWithBrokenOAuth(t *testing.T) {
	tokens := staticMCPTokens{mcpconfig.OAuthConnectionID("broken"): {
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		Expiry:       time.Now().Add(-time.Hour),
		ClientID:     "client",
		TokenURL:     "://bad-token-url",
		AuthStyle:    int(oauth2.AuthStyleInParams),
	}}
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
		{
			ID:        "broken",
			Name:      "Broken",
			URL:       "https://broken.example.com/mcp",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
		{
			ID:        "memory",
			Name:      "Memory",
			URL:       "https://memory.example.com/mcp",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
		},
	}}, tokens, MCPServerPolicyAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(servers))
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(servers[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Name != "Memory" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestEnabledHTTPMCPServersAppliesPolicyBeforeOAuthRefresh(t *testing.T) {
	tokenEndpointCalled := false
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenEndpointCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tokenServer.Close()

	tokens := staticMCPTokens{mcpconfig.OAuthConnectionID("docs"): {
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		Expiry:       time.Now().Add(-time.Hour),
		ClientID:     "client",
		TokenURL:     tokenServer.URL,
		AuthStyle:    int(oauth2.AuthStyleInParams),
	}}
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
	}}, tokens, MCPServerPolicyMemorySearchWorker)
	if err != nil {
		t.Fatal(err)
	}
	if tokenEndpointCalled {
		t.Fatal("filtered MCP server triggered OAuth refresh")
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
	}}, nil, MCPServerPolicyWidget)
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
	}}}, nil, "unknown-policy")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers = %d, want none for unknown policy", len(servers))
	}
}

func TestEnabledHTTPMCPServersKeepsConfiguredAuthorization(t *testing.T) {
	servers, err := enabledHTTPMCPServers(context.Background(), staticMCPServerStore{servers: []mcpconfig.Server{
		{
			ID:        "n8n",
			Name:      "n8n",
			URL:       "https://mcp.example.com/mcp",
			Enabled:   true,
			Transport: mcpconfig.TransportStreamableHTTP,
			Headers:   []mcpconfig.Header{{Name: "Authorization", Value: "Bearer configured"}},
		},
	}}, staticMCPTokens{mcpconfig.OAuthConnectionID("n8n"): {AccessToken: "oauth-token"}}, MCPServerPolicyAll)
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
	if headers["Authorization"] != "Bearer configured" {
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
	}}, nil, MCPServerPolicyAll)
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
