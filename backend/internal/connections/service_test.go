package connections

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestServiceReportsGmailConnectionState(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{}, NewQRService())
	plugin, ok, err := service.Plugin(context.Background(), gmailconnector.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusNotConnected || len(plugin.Connection.Accounts) != 0 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
}

func TestServiceReturnsSavedGmailAccounts(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{
		connections: []integrations.Connection{{
			ID:          gmailconnector.OAuthConnectionID,
			Provider:    gmailconnector.ProviderID,
			AccountID:   "augustinas@example.com",
			AccountName: "Augustinas",
			Alias:       "personal",
			Scopes:      []string{gmailconnector.ScopeModify},
		}},
	}, NewQRService())
	plugin, ok, err := service.Plugin(context.Background(), gmailconnector.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusConnected || len(plugin.Connection.Accounts) != 1 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
	if plugin.Connection.Accounts[0].AccountID != "augustinas@example.com" || plugin.Connection.Accounts[0].Alias != "personal" {
		t.Fatalf("account = %#v", plugin.Connection.Accounts[0])
	}
}

func TestServiceReportsMissingChatSessionAdapter(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{}, NewQRService())
	plugin, ok, err := service.Plugin(context.Background(), whatsapp.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Implementation.Status != "adapter_required" {
		t.Fatalf("implementation = %#v", plugin.Implementation)
	}
}

func TestServiceListsMissingChatSessionAdaptersInCatalog(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{}, NewQRService())
	plugins, err := service.ListPlugins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := pluginIDs(plugins)
	for _, provider := range []string{whatsapp.ProviderID, telegram.ProviderID} {
		if !slices.Contains(ids, provider) {
			t.Fatalf("%s missing from catalog: %v", provider, ids)
		}
	}
}

func TestServiceDoesNotTrustStaticAvailableStatusForSessionPlugins(t *testing.T) {
	catalog := &Catalog{plugins: []integrations.Plugin{{
		ID: "matrix",
		Provider: integrations.Provider{
			ID:   "matrix",
			Name: "Matrix",
		},
		Auth: []integrations.AuthOption{{Kind: integrations.AuthKindSession}},
		Implementation: integrations.Implementation{
			Status: "available",
		},
	}}}
	service := NewService(catalog, &serviceStore{}, NewQRService())

	plugin, ok, err := service.Plugin(context.Background(), "matrix")
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Implementation.Status != "adapter_required" {
		t.Fatalf("implementation = %#v", plugin.Implementation)
	}
	plugins, err := service.ListPlugins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].Implementation.Status != "adapter_required" {
		t.Fatalf("plugins = %#v", plugins)
	}
}

func TestServiceMarksChatSessionPluginsAvailableWhenQRProvidersExist(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{}, NewQRService(
		fakeQRProvider{provider: telegram.ProviderID, expires: time.Now().Add(time.Minute)},
		fakeQRProvider{provider: whatsapp.ProviderID, expires: time.Now().Add(time.Minute)},
	))
	for _, provider := range []string{telegram.ProviderID, whatsapp.ProviderID} {
		plugin, ok, err := service.Plugin(context.Background(), provider)
		if err != nil || !ok {
			t.Fatalf("%s plugin ok=%v err=%v", provider, ok, err)
		}
		if plugin.Implementation.Status != "available" {
			t.Fatalf("%s implementation = %#v", provider, plugin.Implementation)
		}
	}
}

func TestServiceListsChatSessionPluginsWhenQRProvidersExist(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{}, NewQRService(
		fakeQRProvider{provider: telegram.ProviderID, expires: time.Now().Add(time.Minute)},
		fakeQRProvider{provider: whatsapp.ProviderID, expires: time.Now().Add(time.Minute)},
	))
	plugins, err := service.ListPlugins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := pluginIDs(plugins)
	for _, provider := range []string{telegram.ProviderID, whatsapp.ProviderID} {
		if !slices.Contains(ids, provider) {
			t.Fatalf("%s missing from catalog: %v", provider, ids)
		}
	}
}

func TestServiceDisconnectAccount(t *testing.T) {
	store := serviceStore{
		connections: []integrations.Connection{{
			ID:       "gmail:personal",
			Provider: gmailconnector.ProviderID,
			Alias:    "personal",
		}},
	}
	disconnecter := &fakeSessionDisconnecter{provider: gmailconnector.ProviderID}
	service := NewService(NewCatalog(), &store, NewQRService(), disconnecter)
	if err := service.DisconnectAccount(context.Background(), " gmail:personal "); err != nil {
		t.Fatal(err)
	}
	if len(store.connections) != 0 {
		t.Fatalf("connections = %#v", store.connections)
	}
	if disconnecter.connection.ID != "gmail:personal" {
		t.Fatalf("disconnecter connection = %#v", disconnecter.connection)
	}
	if err := service.DisconnectAccount(context.Background(), "gmail:missing"); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceDisconnectCleanupSurvivesCanceledRequest(t *testing.T) {
	store := serviceStore{
		connections: []integrations.Connection{{
			ID:       "telegram:personal",
			Provider: telegram.ProviderID,
		}},
	}
	disconnecter := &fakeSessionDisconnecter{provider: telegram.ProviderID}
	ctx, cancel := context.WithCancel(context.Background())
	store.afterDelete = cancel
	service := NewService(NewCatalog(), &store, NewQRService(), disconnecter)

	if err := service.DisconnectAccount(ctx, "telegram:personal"); err != nil {
		t.Fatal(err)
	}
	if disconnecter.ctxErr != nil {
		t.Fatalf("disconnect cleanup context was canceled: %v", disconnecter.ctxErr)
	}
}

type serviceStore struct {
	connections []integrations.Connection
	afterDelete func()
}

func pluginIDs(plugins []integrations.Plugin) []string {
	ids := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		ids = append(ids, plugin.ID)
	}
	return ids
}

func (s serviceStore) LoadConnection(_ context.Context, id string) (integrations.Connection, bool, error) {
	for _, connection := range s.connections {
		if connection.ID == id {
			return connection, true, nil
		}
	}
	return integrations.Connection{}, false, nil
}

func (s serviceStore) ListConnections(_ context.Context, provider string) ([]integrations.Connection, error) {
	var out []integrations.Connection
	for _, connection := range s.connections {
		if connection.Provider == provider {
			out = append(out, connection)
		}
	}
	return out, nil
}

func (s *serviceStore) DeleteConnection(_ context.Context, id string) (bool, error) {
	for i, connection := range s.connections {
		if connection.ID == id {
			s.connections = append(s.connections[:i], s.connections[i+1:]...)
			if s.afterDelete != nil {
				s.afterDelete()
			}
			return true, nil
		}
	}
	return false, nil
}

type fakeSessionDisconnecter struct {
	provider   string
	connection integrations.Connection
	ctxErr     error
}

func (d *fakeSessionDisconnecter) ProviderID() string {
	return d.provider
}

func (d *fakeSessionDisconnecter) Disconnect(ctx context.Context, connection integrations.Connection) error {
	d.connection = connection
	d.ctxErr = ctx.Err()
	return nil
}
