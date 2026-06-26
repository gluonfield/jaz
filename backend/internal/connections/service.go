package connections

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
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

type Service struct {
	catalog       *Catalog
	store         Store
	qr            *QRService
	disconnecters map[string]SessionDisconnecter
}

func NewService(catalog *Catalog, store Store, qr *QRService, disconnecters ...SessionDisconnecter) *Service {
	service := &Service{
		catalog:       catalog,
		store:         store,
		qr:            qr,
		disconnecters: map[string]SessionDisconnecter{},
	}
	for _, disconnecter := range disconnecters {
		if disconnecter != nil {
			service.disconnecters[disconnecter.ProviderID()] = disconnecter
		}
	}
	return service
}

func (s *Service) ListPlugins(ctx context.Context) ([]integrations.Plugin, error) {
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

func (s *Service) Plugin(ctx context.Context, id string) (integrations.Plugin, bool, error) {
	plugin, ok := s.catalog.Plugin(id)
	if !ok {
		return integrations.Plugin{}, false, nil
	}
	plugin, err := s.withConnection(ctx, plugin)
	return plugin, true, err
}

func (s *Service) DisconnectAccount(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrConnectionNotFound
	}
	connection, ok, err := s.store.LoadConnection(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrConnectionNotFound
	}
	ok, err = s.store.DeleteConnection(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrConnectionNotFound
	}
	if disconnecter := s.disconnecters[connection.Provider]; disconnecter != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), disconnectCleanupTimeout)
		defer cancel()
		return disconnecter.Disconnect(cleanupCtx, connection)
	}
	return nil
}

func (s *Service) withConnection(ctx context.Context, plugin integrations.Plugin) (integrations.Plugin, error) {
	plugin = s.withConnectability(plugin)
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

func (s *Service) withConnectability(plugin integrations.Plugin) integrations.Plugin {
	if len(plugin.Auth) == 0 || plugin.Auth[0].Kind != integrations.AuthKindSession {
		return plugin
	}
	if s.qr != nil && s.qr.Available(plugin.ID) {
		plugin.Implementation.Status = "available"
		return plugin
	}
	plugin.Implementation.Status = "adapter_required"
	return plugin
}
