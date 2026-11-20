package app

import (
	"context"
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/connectors/deployink"
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

func TestConnectionMCPServerReaderInjectsOAuthBackedDeployink(t *testing.T) {
	connectionID := "deployink:mcp-deployink-com"
	reader := connectionReader(fakeConnectionTokenStore{
		connections: map[string][]integrations.Connection{
			deployink.ProviderID: {{
				ID:          connectionID,
				Provider:    deployink.ProviderID,
				AccountName: deployink.ProviderName,
			}},
		},
	})

	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %#v", servers)
	}
	got := servers[0]
	if got.ID != connectionID || got.URL != deployink.RemoteMCPURL || got.Name != deployink.ProviderName || !got.Enabled {
		t.Fatalf("server = %#v", got)
	}
	if len(got.Headers) != 0 {
		t.Fatalf("headers = %#v", got.Headers)
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

func TestZoomConnectionRetainsClientAndManualServers(t *testing.T) {
	manual := mcpconfig.Server{ID: "manual-zoom", Name: "Zoom", URL: "https://mcp.zoom.us/", OAuth: mcpconfig.OAuthConfig{ClientID: "custom-client"}}
	reader := connectionReader(fakeConnectionTokenStore{
		servers:     []mcpconfig.Server{manual},
		connections: map[string][]integrations.Connection{"zoom": {{ID: "zoom:account", Provider: "zoom", AccountName: "Zoom"}}},
	})
	client := mcpconfig.OAuthConfig{ClientID: "registered-client", ClientSecretEnvVar: "ZOOM_TEST_SECRET", UseAppScopes: true}
	reader.catalog = connections.NewCatalog(map[string]mcpconfig.OAuthConfig{"zoom": client})
	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0].OAuth != manual.OAuth || servers[0].URL != manual.URL {
		t.Fatalf("manual server changed: %#v", servers)
	}
	managed := servers[1]
	if managed.ID != "zoom:account" || managed.OAuth != client || managed.Source != mcpconfig.SourceConnection {
		t.Fatalf("managed server = %#v", managed)
	}
}
