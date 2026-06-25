package connections

import (
	"context"

	"github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type Store interface {
	LoadToken(context.Context, string) (integrationoauth.Token, bool, error)
	ListConnections(context.Context, string) ([]integrations.Connection, error)
}

type Service struct {
	catalog *Catalog
	store   Store
}

func NewService(catalog *Catalog, store Store) *Service {
	return &Service{catalog: catalog, store: store}
}

func (s *Service) ListPlugins(ctx context.Context) ([]integrations.Plugin, error) {
	plugins := s.catalog.ListPlugins()
	for i := range plugins {
		plugin, err := s.withConnection(ctx, plugins[i])
		if err != nil {
			return nil, err
		}
		plugins[i] = plugin
	}
	return plugins, nil
}

func (s *Service) Plugin(ctx context.Context, id string) (integrations.Plugin, bool, error) {
	plugin, ok := s.catalog.Plugin(id)
	if !ok {
		return integrations.Plugin{}, false, nil
	}
	plugin, err := s.withConnection(ctx, plugin)
	return plugin, true, err
}

func (s *Service) withConnection(ctx context.Context, plugin integrations.Plugin) (integrations.Plugin, error) {
	if plugin.ID != gmail.ProviderID {
		return plugin, nil
	}
	accounts, err := s.store.ListConnections(ctx, gmail.ProviderID)
	if err != nil {
		return integrations.Plugin{}, err
	}
	connection := integrations.PluginConnection{Status: integrations.PluginConnectionStatusNotConnected}
	if len(accounts) > 0 {
		connection.Status = integrations.PluginConnectionStatusConnected
		connection.Accounts = accounts
		plugin.Connection = &connection
		return plugin, nil
	}
	token, ok, err := s.store.LoadToken(ctx, gmail.OAuthConnectionID)
	if err != nil {
		return integrations.Plugin{}, err
	}
	if ok && (token.AccessToken != "" || token.RefreshToken != "") {
		connection.Status = integrations.PluginConnectionStatusConnected
		connection.Accounts = []integrations.Connection{{
			ID:       gmail.OAuthConnectionID,
			Provider: gmail.ProviderID,
			Alias:    "default",
			Scopes:   token.Scopes,
		}}
	}
	plugin.Connection = &connection
	return plugin, nil
}
