package connections

import (
	"context"
	"testing"

	"github.com/wins/jaz/backend/internal/connectors/deployink"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

func TestRemoteMCPConnectorCreatesAndUpdatesDeployinkServer(t *testing.T) {
	store := &remoteMCPStore{}
	connector := NewRemoteMCPConnector(store, nil)

	start, err := connector.Connect(context.Background(), deployink.Plugin())
	if err != nil {
		t.Fatal(err)
	}
	if start.ServerID == "" || start.Name != deployink.ProviderName || start.URL != deployink.RemoteMCPURL {
		t.Fatalf("start = %#v", start)
	}
	if len(store.servers) != 1 || !store.servers[0].Enabled {
		t.Fatalf("servers = %#v", store.servers)
	}

	second, err := connector.Connect(context.Background(), deployink.Plugin())
	if err != nil {
		t.Fatal(err)
	}
	if second.ServerID != start.ServerID || len(store.servers) != 1 {
		t.Fatalf("second=%#v servers=%#v", second, store.servers)
	}
}

func TestRemoteMCPConnectorPreservesExistingServerAuthConfig(t *testing.T) {
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:                "mcp_deployink",
		Name:              "Ink",
		Transport:         mcpconfig.TransportStreamableHTTP,
		URL:               deployink.RemoteMCPURL + "/",
		BearerTokenEnvVar: "INK_API_KEY",
		Headers:           []mcpconfig.Header{{Name: "X-Team", Value: "platform"}},
		OAuth:             mcpconfig.OAuthConfig{ClientID: "ink-client", Issuer: deployink.RemoteMCPURL},
	}}}
	connector := NewRemoteMCPConnector(store, nil)

	start, err := connector.Connect(context.Background(), deployink.Plugin())
	if err != nil {
		t.Fatal(err)
	}
	if start.ServerID != "mcp_deployink" || start.Name != deployink.ProviderName {
		t.Fatalf("start = %#v", start)
	}
	server := store.servers[0]
	if !server.Enabled || server.Name != deployink.ProviderName || server.URL != deployink.RemoteMCPURL+"/" {
		t.Fatalf("server = %#v", server)
	}
	if server.BearerTokenEnvVar != "INK_API_KEY" ||
		len(server.Headers) != 1 || server.Headers[0].Name != "X-Team" ||
		server.OAuth.ClientID != "ink-client" || server.OAuth.Issuer != deployink.RemoteMCPURL {
		t.Fatalf("server auth config = %#v", server)
	}
}

func TestRemoteMCPConnectorReportsConnectionFromMCPServer(t *testing.T) {
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:        "mcp_deployink",
		Name:      deployink.ProviderName,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       deployink.RemoteMCPURL,
		Enabled:   true,
	}}}
	service := NewService(NewCatalog(), &serviceStore{}, NewRemoteMCPConnector(store, nil))

	plugin, ok, err := service.Plugin(context.Background(), deployink.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != "connected" || len(plugin.Connection.Accounts) != 1 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
	account := plugin.Connection.Accounts[0]
	if account.ID != "mcp_deployink" || account.Provider != deployink.ProviderID || account.AccountID != deployink.RemoteMCPURL {
		t.Fatalf("account = %#v", account)
	}
}

func TestRemoteMCPConnectorReportsDisabledServerAsNotConnected(t *testing.T) {
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:        "mcp_deployink",
		Name:      deployink.ProviderName,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       deployink.RemoteMCPURL,
	}}}
	service := NewService(NewCatalog(), &serviceStore{}, NewRemoteMCPConnector(store, nil))

	plugin, ok, err := service.Plugin(context.Background(), deployink.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != "not_connected" || len(plugin.Connection.Accounts) != 0 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
}

func TestRemoteMCPConnectorDisconnectDeletesPluginServer(t *testing.T) {
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:        "mcp_deployink",
		Name:      deployink.ProviderName,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       deployink.RemoteMCPURL,
		Enabled:   true,
	}}}
	service := NewService(NewCatalog(), &serviceStore{}, NewRemoteMCPConnector(store, nil))

	if err := service.DisconnectAccount(context.Background(), "mcp_deployink"); err != nil {
		t.Fatal(err)
	}
	if len(store.servers) != 0 {
		t.Fatalf("servers = %#v", store.servers)
	}
}

type remoteMCPStore struct {
	servers []mcpconfig.Server
	nextID  int
}

func (s *remoteMCPStore) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), s.servers...), nil
}

func (s *remoteMCPStore) CreateMCPServer(input mcpconfig.ServerInput) (mcpconfig.Server, error) {
	s.nextID++
	server := mcpconfig.Server{
		ID:                "mcp_test",
		Name:              input.Name,
		Transport:         mcpconfig.TransportStreamableHTTP,
		URL:               input.URL,
		Enabled:           input.Enabled,
		BearerTokenEnvVar: input.BearerTokenEnvVar,
		Headers:           input.Headers,
		OAuth:             input.OAuth,
	}
	if s.nextID > 1 {
		server.ID = "mcp_test_2"
	}
	s.servers = append(s.servers, server)
	return server, nil
}

func (s *remoteMCPStore) UpdateMCPServer(id string, input mcpconfig.ServerInput) (mcpconfig.Server, error) {
	for i := range s.servers {
		if s.servers[i].ID == id {
			s.servers[i].Name = input.Name
			s.servers[i].URL = input.URL
			s.servers[i].Enabled = input.Enabled
			s.servers[i].BearerTokenEnvVar = input.BearerTokenEnvVar
			s.servers[i].Headers = input.Headers
			s.servers[i].OAuth = input.OAuth
			return s.servers[i], nil
		}
	}
	return mcpconfig.Server{}, nil
}

func (s *remoteMCPStore) DeleteMCPServer(id string) error {
	for i := range s.servers {
		if s.servers[i].ID == id {
			s.servers = append(s.servers[:i], s.servers[i+1:]...)
			return nil
		}
	}
	return nil
}
