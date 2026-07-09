package connections

import (
	"context"
	"errors"
	"fmt"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type MCPConnectionAuthorizer interface {
	AuthorizeWithHook(context.Context, mcpconfig.Server, mcpconfig.AuthorizeOptions, func(context.Context) error) mcpconfig.ServerStatus
}

type MCPConnectionStore interface {
	SaveConnection(context.Context, integrations.Connection) error
}

type MCPConnectionConnector struct {
	store      MCPConnectionStore
	authorizer MCPConnectionAuthorizer
}

func NewMCPConnectionConnector(store MCPConnectionStore, authorizer MCPConnectionAuthorizer) *MCPConnectionConnector {
	return &MCPConnectionConnector{store: store, authorizer: authorizer}
}

func (c *MCPConnectionConnector) Connect(ctx context.Context, plugin integrations.Plugin, redirectURL string) (OAuthStart, error) {
	connection, server, err := mcpConnectionSpec(plugin)
	if err != nil {
		return OAuthStart{}, err
	}
	status := c.authorizer.AuthorizeWithHook(ctx, server, mcpconfig.AuthorizeOptions{
		RedirectURL:   redirectURL,
		ReturnAuthURL: true,
	}, func(ctx context.Context) error {
		return c.store.SaveConnection(ctx, connection)
	})
	if status.AuthURL != "" {
		return OAuthStart{AuthURL: status.AuthURL}, nil
	}
	if status.Error != "" {
		return OAuthStart{}, errors.New(status.Error)
	}
	return OAuthStart{}, fmt.Errorf("connection plugin %q did not return an authorization URL", plugin.ID)
}

func mcpConnectionSpec(plugin integrations.Plugin) (integrations.Connection, mcpconfig.Server, error) {
	if !mcpConnectionPlugin(plugin) {
		return integrations.Connection{}, mcpconfig.Server{}, fmt.Errorf("connection plugin %q is not an MCP-backed connection", plugin.ID)
	}
	input, err := mcpconfig.ValidateInput(mcpconfig.ServerInput{
		Name:    plugin.Name,
		URL:     plugin.RemoteMCP.URL,
		Enabled: true,
	})
	if err != nil {
		return integrations.Connection{}, mcpconfig.Server{}, err
	}
	providerID := plugin.Provider.ID
	if providerID == "" {
		providerID = plugin.ID
	}
	id, err := integrations.ConnectionID(providerID, input.URL)
	if err != nil {
		return integrations.Connection{}, mcpconfig.Server{}, err
	}
	name := plugin.Provider.Name
	if name == "" {
		name = plugin.Name
	}
	connection := integrations.Connection{
		ID:          id,
		Provider:    providerID,
		AccountID:   input.URL,
		AccountName: name,
	}
	server := mcpconfig.Server{
		ID:        id,
		Name:      input.Name,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       input.URL,
		Enabled:   true,
	}
	return connection, server, nil
}

func mcpConnectionPlugin(plugin integrations.Plugin) bool {
	return plugin.PrimaryAuthKind() == integrations.AuthKindMCPConnection && plugin.RemoteMCP != nil
}
