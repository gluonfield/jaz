package connections

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

var ErrConnectionNotFound = errors.New("connection account not found")

const disconnectCleanupTimeout = 10 * time.Second

type Store interface {
	LoadConnection(context.Context, string) (integrations.Connection, bool, error)
	ListConnections(context.Context, string) ([]integrations.Connection, error)
	DeleteConnection(context.Context, string) (bool, error)
}

type SessionDisconnecter interface {
	ProviderID() string
	Disconnect(context.Context, integrations.Connection) error
}

type DisconnectResult struct {
	MCPServersChanged bool
}

type Service struct {
	catalog       *Catalog
	store         Store
	disconnecters map[string]SessionDisconnecter
}

func NewService(catalog *Catalog, store Store, disconnecters ...SessionDisconnecter) *Service {
	service := &Service{
		catalog:       catalog,
		store:         store,
		disconnecters: map[string]SessionDisconnecter{},
	}
	for _, disconnecter := range disconnecters {
		if disconnecter != nil {
			service.disconnecters[disconnecter.ProviderID()] = disconnecter
		}
	}
	return service
}

func (s *Service) Catalog() *Catalog {
	return s.catalog
}

func (s *Service) ListPlugins(ctx context.Context, _ bool) ([]integrations.Plugin, error) {
	plugins := s.catalog.ListPlugins()
	out := make([]integrations.Plugin, 0, len(plugins))
	for i := range plugins {
		plugin, err := s.withConnection(ctx, plugins[i])
		if err != nil {
			return nil, err
		}
		out = append(out, plugin)
	}
	return out, nil
}

func (s *Service) Plugin(ctx context.Context, id string, _ bool) (integrations.Plugin, bool, error) {
	plugin, ok := s.catalog.Plugin(id)
	if !ok {
		return integrations.Plugin{}, false, nil
	}
	plugin, err := s.withConnection(ctx, plugin)
	return plugin, true, err
}

func (s *Service) ListAccounts(ctx context.Context, providerID string) ([]integrations.Connection, error) {
	if s == nil || s.store == nil || providerID == "" {
		return nil, nil
	}
	return s.store.ListConnections(ctx, providerID)
}

func (s *Service) DisconnectAccount(ctx context.Context, id string) (DisconnectResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return DisconnectResult{}, ErrConnectionNotFound
	}
	connection, ok, err := s.store.LoadConnection(ctx, id)
	if err != nil {
		return DisconnectResult{}, err
	}
	if !ok {
		return DisconnectResult{}, ErrConnectionNotFound
	}
	unlock := integrationoauth.LockToken(id)
	unlockMCP := integrationoauth.LockToken(mcpconfig.OAuthConnectionID(id))
	ok, err = s.store.DeleteConnection(ctx, id)
	unlockMCP()
	unlock()
	if err != nil {
		return DisconnectResult{}, err
	}
	if !ok {
		return DisconnectResult{}, ErrConnectionNotFound
	}
	usesConnectionMCP := s.providerUsesConnectionMCP(connection.Provider)
	if disconnecter := s.disconnecters[connection.Provider]; disconnecter != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), disconnectCleanupTimeout)
		defer cancel()
		if err := disconnecter.Disconnect(cleanupCtx, connection); err != nil {
			return DisconnectResult{}, err
		}
	}
	return DisconnectResult{MCPServersChanged: usesConnectionMCP}, nil
}

// AgentConnections lists every connected account across native integrations.
func (s *Service) AgentConnections(ctx context.Context) ([]AgentConnection, error) {
	plugins, err := s.ListPlugins(ctx, false)
	if err != nil {
		return nil, err
	}
	var out []AgentConnection
	for _, plugin := range plugins {
		if plugin.Connection == nil {
			continue
		}
		connection := AgentConnection{ProviderID: plugin.Provider.ID, ProviderName: plugin.Provider.Name}
		if connection.ProviderName == "" {
			connection.ProviderName = plugin.Name
		}
		for _, account := range plugin.Connection.Accounts {
			connection.Account = AccountLabel(account)
			connection.RelevantPaths = s.relevantPaths(account)
			out = append(out, connection)
		}
	}
	return out, nil
}

func (s *Service) withConnection(ctx context.Context, plugin integrations.Plugin) (integrations.Plugin, error) {
	if plugin.Provider.ID == "" {
		return plugin, nil
	}
	accounts, err := s.store.ListConnections(ctx, plugin.Provider.ID)
	if err != nil {
		return integrations.Plugin{}, err
	}
	connection := integrations.PluginConnection{Status: integrations.PluginConnectionStatusNotConnected}
	if len(accounts) > 0 {
		connection.Status = integrations.PluginConnectionStatusConnected
		connection.Accounts = accounts
	}
	plugin.Connection = &connection
	return plugin, nil
}

func (s *Service) providerUsesConnectionMCP(provider string) bool {
	plugin, ok := s.catalog.Plugin(provider)
	if !ok {
		return true
	}
	return plugin.UsesConnectionMCP()
}

func AccountLabel(connection integrations.Connection) string {
	label := connection.AccountRef()
	accountID := strings.TrimSpace(connection.AccountID)
	if label == "" {
		label = accountID
	}
	if label == "" {
		label = strings.TrimSpace(connection.ID)
	}
	if accountID != "" && label != accountID {
		return label + " (" + accountID + ")"
	}
	return label
}
