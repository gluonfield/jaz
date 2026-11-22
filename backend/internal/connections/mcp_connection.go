package connections

import (
	"cmp"
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

func (c *MCPConnectionConnector) ConnectServer(ctx context.Context, pluginID, serverID, displayName, rawURL string, client mcpconfig.OAuthConfig, opts mcpconfig.AuthorizeOptions) (OAuthStart, error) {
	input, err := mcpconfig.ValidateInput(mcpconfig.ServerInput{
		Name:    displayName,
		URL:     rawURL,
		Enabled: true,
	})
	if err != nil {
		return OAuthStart{}, err
	}
	connID := serverID
	if connID == "" {
		var idErr error
		connID, idErr = integrations.ConnectionID(pluginID, input.URL)
		if idErr != nil {
			return OAuthStart{}, idErr
		}
		serverID = connID
	}
	connection := integrations.Connection{
		ID:          connID,
		Provider:    pluginID,
		AccountID:   input.URL,
		AccountName: cmp.Or(displayName, pluginID),
	}
	server := mcpconfig.Server{
		ID:        serverID,
		Name:      input.Name,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       input.URL,
		Enabled:   true,
		OAuth:     client,
	}
	opts.ReplacePending = true
	status := c.authorizer.AuthorizeWithHook(ctx, server, opts, func(ctx context.Context) error {
		return c.store.SaveConnection(ctx, connection)
	})
	if status.AuthURL != "" {
		return OAuthStart{AuthURL: status.AuthURL}, nil
	}
	if status.Error != "" {
		return OAuthStart{}, errors.New(status.Error)
	}
	return OAuthStart{}, fmt.Errorf("plugin %q did not return an authorization URL", pluginID)
}
