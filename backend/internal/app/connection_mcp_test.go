package app

import (
	"context"
	"testing"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/connectors/deployink"
	slackconnector "github.com/wins/jaz/backend/internal/connectors/slack"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
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

func connectionReader(store fakeConnectionTokenStore) connectionMCPServerReader {
	return connectionMCPServerReader{store: store, catalog: connections.NewCatalog()}
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
	if len(got.Headers) != 1 || got.Headers[0].Name != "Authorization" || got.Headers[0].Value != "Bearer xoxp-1" {
		t.Fatalf("headers = %#v", got.Headers)
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
