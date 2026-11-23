package app

import (
	"context"
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/connections"
	driveconnector "github.com/wins/jaz/backend/internal/connectors/drive"
	slackconnector "github.com/wins/jaz/backend/internal/connectors/slack"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type fakeConnectionTokenStore struct {
	servers     []mcpconfig.Server
	connections map[string][]integrations.Connection
	tokens      map[string]integrationoauth.Token
}

func (f fakeConnectionTokenStore) ListMCPServers() ([]mcpconfig.Server, error) {
	return f.servers, nil
}

func (f fakeConnectionTokenStore) ListConnections(_ context.Context, provider string) ([]integrations.Connection, error) {
	return f.connections[provider], nil
}

func (f fakeConnectionTokenStore) LoadToken(_ context.Context, id string) (integrationoauth.Token, bool, error) {
	token, ok := f.tokens[id]
	return token, ok, nil
}

func (f fakeConnectionTokenStore) SaveToken(_ context.Context, id string, token integrationoauth.Token) error {
	f.tokens[id] = token
	return nil
}

func connectionReader(store fakeConnectionTokenStore) connectionMCPServerReader {
	return connectionMCPServerReader{store: store, catalog: connections.NewCatalog(nil)}
}

func TestConnectionMCPServerReaderInjectsTokenBackedSlack(t *testing.T) {
	reader := connectionReader(fakeConnectionTokenStore{
		connections: map[string][]integrations.Connection{
			"slack": {{ID: "slack:acme-u1", Provider: "slack", Alias: "acme-augustinas"}},
		},
		tokens: map[string]integrationoauth.Token{"slack:acme-u1": {AccessToken: "xoxp-1"}},
	})

	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %#v", servers)
	}
	got := servers[0]
	if got.ID != "slack:acme-u1" || got.URL != slackconnector.RemoteMCPURL || got.Name != "acme-augustinas" || !got.Enabled {
		t.Fatalf("server = %#v", got)
	}
	if len(got.Headers) != 0 || got.TokenConnectionID != "slack:acme-u1" {
		t.Fatalf("account grant = %q, headers = %#v", got.TokenConnectionID, got.Headers)
	}
}

func TestConnectionMCPServerReaderSkipsSlackWithoutToken(t *testing.T) {
	reader := connectionReader(fakeConnectionTokenStore{
		connections: map[string][]integrations.Connection{
			"slack": {{ID: "slack:acme-u1", Provider: "slack"}},
		},
	})

	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers = %#v", servers)
	}
}

func TestConnectionMCPServerReaderIgnoresProvidersWithoutConnectionBackedMCP(t *testing.T) {
	reader := connectionReader(fakeConnectionTokenStore{
		connections: map[string][]integrations.Connection{
			"gmail": {{ID: "gmail:default", Provider: "gmail"}},
		},
		tokens: map[string]integrationoauth.Token{"gmail:default": {AccessToken: "ya29"}},
	})

	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers = %#v", servers)
	}
}

func TestConnectionMCPServerReaderKeepsDriveAccountsAndToolsDistinct(t *testing.T) {
	store := fakeConnectionTokenStore{
		connections: map[string][]integrations.Connection{
			driveconnector.ProviderID: {
				{ID: "drive:personal", Provider: driveconnector.ProviderID, Alias: "personal"},
				{ID: "drive:work", Provider: driveconnector.ProviderID, Alias: "work"},
			},
		},
		tokens: map[string]integrationoauth.Token{
			"drive:personal": {AccessToken: "personal-grant"},
			"drive:work":     {AccessToken: "work-grant"},
		},
	}
	reader := connectionReader(store)
	servers, err := reader.ListMCPServers()
	if err != nil || len(servers) != 2 {
		t.Fatalf("Drive inventory = %+v, %v", servers, err)
	}
	registry := tools.NewRegistry()
	manager := mcpruntime.NewManager(reader, nil, registry, log.New(io.Discard))
	defer manager.Close()
	manager.Refresh(t.Context())
	for _, server := range servers {
		if server.Source != mcpconfig.SourceConnection || server.TokenConnectionID != server.ID || server.URL != "" {
			t.Fatalf("Drive account provenance = %+v", server)
		}
		if status := manager.Status(server.ID); status.Status != "connected" || status.ToolCount != 8 {
			t.Fatalf("Drive tool catalog = %+v", status)
		}
	}
	if count := len(registry.Definitions()); count != 16 {
		t.Fatalf("distinct account tool count = %d", count)
	}
}

func TestConnectionMCPServerReaderSkipsDisabledPlugin(t *testing.T) {
	reader := connectionReader(fakeConnectionTokenStore{
		connections: map[string][]integrations.Connection{
			"slack": {{ID: "slack:acme-u1", Provider: "slack", Alias: "acme"}},
		},
		tokens: map[string]integrationoauth.Token{"slack:acme-u1": {AccessToken: "xoxp-1"}},
	})
	reader.enabled = func(_ context.Context, id string) bool { return id != "slack" }
	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("disabled plugin emitted servers = %#v", servers)
	}
}

func TestConnectionMCPServerReaderIsolatesCloudWorkspaceFromLocalMCP(t *testing.T) {
	store := fakeConnectionTokenStore{
		servers: []mcpconfig.Server{
			{ID: "local-mcp", Name: "local-mcp", URL: "https://local.example/mcp", Enabled: true},
		},
		connections: map[string][]integrations.Connection{
			"slack": {{ID: "slack:personal", Provider: "slack", Alias: "personal"}},
		},
		tokens: map[string]integrationoauth.Token{"slack:personal": {AccessToken: "xoxp-personal"}},
	}
	cloudActive := true
	reader := connectionMCPServerReader{
		store:   store,
		catalog: connections.NewCatalog(nil),
		cloudMCP: func(context.Context) (mcpconfig.Server, bool) {
			if !cloudActive {
				return mcpconfig.Server{}, false
			}
			return mcpconfig.Server{
				ID:      "jaz_cloud",
				Name:    "jaz_cloud",
				URL:     "https://cloud.example/v1/mcp",
				Enabled: true,
			}, true
		},
	}

	// Active Cloud workspace exposes ONLY Cloud MCP (suppresses local custom MCP + personal Slack).
	cloudServers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(cloudServers) != 1 || cloudServers[0].ID != "jaz_cloud" {
		t.Fatalf("cloud workspace MCP servers = %#v, want only jaz_cloud", cloudServers)
	}

	// Local workspace exposes local MCP + personal Slack, and NO Cloud MCP.
	cloudActive = false
	localServers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(localServers) != 2 {
		t.Fatalf("local workspace MCP servers = %#v, want local-mcp + slack:personal", localServers)
	}
	for _, s := range localServers {
		if s.ID == "jaz_cloud" {
			t.Fatalf("local workspace must not attach jaz_cloud: %#v", localServers)
		}
	}
}
