package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type RemoteMCPStart struct {
	ServerID string `json:"server_id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

type RemoteMCPStore interface {
	ListMCPServers() ([]mcpconfig.Server, error)
	CreateMCPServer(mcpconfig.ServerInput) (mcpconfig.Server, error)
	UpdateMCPServer(string, mcpconfig.ServerInput) (mcpconfig.Server, error)
	DeleteMCPServer(string) error
}

type RemoteMCPConnector struct {
	store RemoteMCPStore
}

func NewRemoteMCPConnector(store RemoteMCPStore) *RemoteMCPConnector {
	return &RemoteMCPConnector{store: store}
}

func (c *RemoteMCPConnector) Connect(ctx context.Context, plugin integrations.Plugin) (RemoteMCPStart, error) {
	if c == nil || c.store == nil {
		return RemoteMCPStart{}, errors.New("remote MCP connections are not configured")
	}
	input, err := remoteMCPServerInput(plugin)
	if err != nil {
		return RemoteMCPStart{}, err
	}
	servers, err := c.store.ListMCPServers()
	if err != nil {
		return RemoteMCPStart{}, err
	}
	var server mcpconfig.Server
	for _, current := range servers {
		if sameMCPServerURL(current.URL, input.URL) {
			input.URL = current.URL
			input.BearerTokenEnvVar = current.BearerTokenEnvVar
			input.Headers = current.Headers
			input.OAuth = current.OAuth
			server, err = c.store.UpdateMCPServer(current.ID, input)
			if err != nil {
				return RemoteMCPStart{}, err
			}
			return remoteMCPStart(server), nil
		}
	}
	server, err = c.store.CreateMCPServer(input)
	if err != nil {
		return RemoteMCPStart{}, err
	}
	return remoteMCPStart(server), nil
}

func (c *RemoteMCPConnector) Connection(ctx context.Context, plugin integrations.Plugin) (*integrations.PluginConnection, error) {
	if !remoteMCPConnectionPlugin(plugin) {
		return nil, nil
	}
	if c == nil || c.store == nil {
		return &integrations.PluginConnection{Status: integrations.PluginConnectionStatusNotConnected}, nil
	}
	input, err := remoteMCPServerInput(plugin)
	if err != nil {
		return nil, err
	}
	servers, err := c.store.ListMCPServers()
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		if !sameMCPServerURL(server.URL, input.URL) {
			continue
		}
		connection := integrations.PluginConnection{Status: integrations.PluginConnectionStatusNotConnected}
		if server.Enabled {
			connection.Status = integrations.PluginConnectionStatusConnected
			connection.Accounts = []integrations.Connection{{
				ID:          server.ID,
				Provider:    plugin.Provider.ID,
				AccountID:   server.URL,
				AccountName: server.Name,
				Alias:       "default",
			}}
		}
		return &connection, nil
	}
	return &integrations.PluginConnection{Status: integrations.PluginConnectionStatusNotConnected}, nil
}

func (c *RemoteMCPConnector) Disconnect(ctx context.Context, id string, catalog *Catalog) (bool, error) {
	if c == nil || c.store == nil || catalog == nil {
		return false, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	servers, err := c.store.ListMCPServers()
	if err != nil {
		return false, err
	}
	for _, server := range servers {
		if server.ID != id || !catalogHasRemoteMCPURL(catalog, server.URL) {
			continue
		}
		if err := c.store.DeleteMCPServer(id); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func remoteMCPServerInput(plugin integrations.Plugin) (mcpconfig.ServerInput, error) {
	if !remoteMCPConnectionPlugin(plugin) {
		return mcpconfig.ServerInput{}, fmt.Errorf("connection plugin %q is not a remote MCP connection", plugin.ID)
	}
	return mcpconfig.ValidateInput(mcpconfig.ServerInput{
		Name:    plugin.Name,
		URL:     plugin.RemoteMCP.URL,
		Enabled: true,
	})
}

func remoteMCPConnectionPlugin(plugin integrations.Plugin) bool {
	return plugin.PrimaryAuthKind() == integrations.AuthKindRemoteMCP &&
		plugin.RemoteMCP != nil &&
		!plugin.UsesConnectionMCP()
}

func remoteMCPStart(server mcpconfig.Server) RemoteMCPStart {
	return RemoteMCPStart{ServerID: server.ID, Name: server.Name, URL: server.URL}
}

func sameMCPServerURL(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), "/"), strings.TrimRight(strings.TrimSpace(b), "/"))
}

func catalogHasRemoteMCPURL(catalog *Catalog, rawURL string) bool {
	for _, plugin := range catalog.ListPlugins() {
		if !remoteMCPConnectionPlugin(plugin) {
			continue
		}
		input, err := remoteMCPServerInput(plugin)
		if err == nil && sameMCPServerURL(rawURL, input.URL) {
			return true
		}
	}
	return false
}
