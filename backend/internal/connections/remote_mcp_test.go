package connections

import (
	"context"
	"testing"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestRemoteMCPConnectorCreatesAndUpdatesPluginServer(t *testing.T) {
	store := &remoteMCPStore{}
	connector := NewRemoteMCPConnector(store)

	plugin := remoteMCPPlugin()
	start, err := connector.Connect(context.Background(), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if start.ServerID == "" || start.Name != plugin.Name || start.URL != plugin.RemoteMCP.URL {
		t.Fatalf("start = %#v", start)
	}
	if len(store.servers) != 1 || !store.servers[0].Enabled {
		t.Fatalf("servers = %#v", store.servers)
	}

	second, err := connector.Connect(context.Background(), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if second.ServerID != start.ServerID || len(store.servers) != 1 {
		t.Fatalf("second=%#v servers=%#v", second, store.servers)
	}
}

func TestRemoteMCPConnectorPreservesExistingServerAuthConfig(t *testing.T) {
	plugin := remoteMCPPlugin()
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:                "mcp_docs",
		Name:              "Ink",
		Transport:         mcpconfig.TransportStreamableHTTP,
		URL:               plugin.RemoteMCP.URL + "/",
		BearerTokenEnvVar: "INK_API_KEY",
		Headers:           []mcpconfig.Header{{Name: "X-Team", Value: "platform"}},
		OAuth:             mcpconfig.OAuthConfig{ClientID: "ink-client", Issuer: plugin.RemoteMCP.URL},
	}}}
	connector := NewRemoteMCPConnector(store)

	start, err := connector.Connect(context.Background(), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if start.ServerID != "mcp_docs" || start.Name != plugin.Name {
		t.Fatalf("start = %#v", start)
	}
	server := store.servers[0]
	if !server.Enabled || server.Name != plugin.Name || server.URL != plugin.RemoteMCP.URL+"/" {
		t.Fatalf("server = %#v", server)
	}
	if server.BearerTokenEnvVar != "INK_API_KEY" ||
		len(server.Headers) != 1 || server.Headers[0].Name != "X-Team" ||
		server.OAuth.ClientID != "ink-client" || server.OAuth.Issuer != plugin.RemoteMCP.URL {
		t.Fatalf("server auth config = %#v", server)
	}
}

func TestRemoteMCPConnectorReportsConnectionFromMCPServer(t *testing.T) {
	plugin := remoteMCPPlugin()
	catalog := &Catalog{plugins: []integrations.Plugin{plugin}}
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:        "mcp_docs",
		Name:      plugin.Name,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       plugin.RemoteMCP.URL,
		Enabled:   true,
	}}}
	service := NewService(catalog, &serviceStore{}, NewRemoteMCPConnector(store))

	plugin, ok, err := service.Plugin(context.Background(), plugin.ID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != "connected" || len(plugin.Connection.Accounts) != 1 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
	account := plugin.Connection.Accounts[0]
	if account.ID != "mcp_docs" || account.Provider != "docs" || account.AccountID != plugin.RemoteMCP.URL {
		t.Fatalf("account = %#v", account)
	}
}

func TestRemoteMCPConnectorReportsDisabledServerAsNotConnected(t *testing.T) {
	plugin := remoteMCPPlugin()
	catalog := &Catalog{plugins: []integrations.Plugin{plugin}}
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:        "mcp_docs",
		Name:      plugin.Name,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       plugin.RemoteMCP.URL,
	}}}
	service := NewService(catalog, &serviceStore{}, NewRemoteMCPConnector(store))

	plugin, ok, err := service.Plugin(context.Background(), plugin.ID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != "not_connected" || len(plugin.Connection.Accounts) != 0 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
}

func TestRemoteMCPConnectorDisconnectDeletesPluginServer(t *testing.T) {
	plugin := remoteMCPPlugin()
	catalog := &Catalog{plugins: []integrations.Plugin{plugin}}
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:        "mcp_docs",
		Name:      plugin.Name,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       plugin.RemoteMCP.URL,
		Enabled:   true,
	}}}
	service := NewService(catalog, &serviceStore{}, NewRemoteMCPConnector(store))

	result, err := service.DisconnectAccount(context.Background(), "mcp_docs")
	if err != nil {
		t.Fatal(err)
	}
	if !result.MCPServersChanged {
		t.Fatalf("result = %#v, want MCPServersChanged", result)
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
