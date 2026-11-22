package connections

import (
	"context"
	"errors"
	"slices"
	"testing"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	slackconnector "github.com/wins/jaz/backend/internal/connectors/slack"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestServiceReportsGmailConnectionState(t *testing.T) {
	service := NewService(NewCatalog(nil), &serviceStore{})
	plugin, ok, err := service.Plugin(context.Background(), gmailconnector.ProviderID, false)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusNotConnected || len(plugin.Connection.Accounts) != 0 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
}

func TestServiceReturnsSavedGmailAccounts(t *testing.T) {
	service := NewService(NewCatalog(nil), &serviceStore{
		connections: []integrations.Connection{{
			ID:          gmailconnector.OAuthConnectionID,
			Provider:    gmailconnector.ProviderID,
			AccountID:   "user@example.com",
			AccountName: "Personal",
			Alias:       "personal",
			Scopes:      []string{gmailconnector.ScopeModify},
		}},
	})
	plugin, ok, err := service.Plugin(context.Background(), gmailconnector.ProviderID, false)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusConnected || len(plugin.Connection.Accounts) != 1 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
	if plugin.Connection.Accounts[0].AccountID != "user@example.com" || plugin.Connection.Accounts[0].Alias != "personal" {
		t.Fatalf("account = %#v", plugin.Connection.Accounts[0])
	}
}

func TestServiceAddsAgentRelevantPaths(t *testing.T) {
	service := NewService(NewCatalog(nil), &serviceStore{
		connections: []integrations.Connection{{
			ID:        "telegram:personal",
			Provider:  telegram.ProviderID,
			AccountID: "42",
			Alias:     "personal",
		}, {
			ID:        "whatsapp:personal",
			Provider:  whatsapp.ProviderID,
			AccountID: "+44 7700 900123",
			Alias:     "personal",
		}, {
			ID:        "gmail:personal",
			Provider:  gmailconnector.ProviderID,
			AccountID: "user@example.com",
			Alias:     "personal",
		}},
	})

	connections, err := service.AgentConnections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	accounts := slices.DeleteFunc(connections, func(connection AgentConnection) bool { return connection.Account == "" })
	if len(accounts) != 3 {
		t.Fatalf("agent accounts = %#v", accounts)
	}
	for _, connection := range accounts {
		switch connection.ProviderID {
		case telegram.ProviderID:
			if len(connection.RelevantPaths) != 2 ||
				connection.RelevantPaths[0].Kind != AgentPathKindMemoryPage ||
				connection.RelevantPaths[0].Path != "sources/chat/telegram/42/contacts.md" ||
				connection.RelevantPaths[1].Kind != AgentPathKindMemoryPrefix ||
				connection.RelevantPaths[1].Path != "sources/chat/telegram/42/conversations/" {
				t.Fatalf("telegram connection = %#v", connection)
			}
		case whatsapp.ProviderID:
			if len(connection.RelevantPaths) != 2 ||
				connection.RelevantPaths[0].Kind != AgentPathKindMemoryPage ||
				connection.RelevantPaths[0].Path != "sources/chat/whatsapp/44-7700-900123/contacts.md" ||
				connection.RelevantPaths[1].Kind != AgentPathKindMemoryPrefix ||
				connection.RelevantPaths[1].Path != "sources/chat/whatsapp/44-7700-900123/conversations/" {
				t.Fatalf("whatsapp connection = %#v", connection)
			}
		case gmailconnector.ProviderID:
			if len(connection.RelevantPaths) != 1 ||
				connection.RelevantPaths[0].Kind != AgentPathKindMemoryPrefix ||
				connection.RelevantPaths[0].Path != "sources/email/gmail/user-example-com/messages/" {
				t.Fatalf("gmail connection = %#v", connection)
			}
		}
	}
}

func TestServiceListsOnlyConnectedAccountsToAgents(t *testing.T) {
	service := NewService(NewCatalog(nil), &serviceStore{
		connections: []integrations.Connection{{ID: gmailconnector.OAuthConnectionID, Provider: gmailconnector.ProviderID, AccountID: "user@example.com"}},
	})
	connections, err := service.AgentConnections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].ProviderID != gmailconnector.ProviderID || connections[0].Account == "" {
		t.Fatalf("agent connections = %#v, want the one connected Gmail account", connections)
	}
}

func TestServiceLeavesChatPluginConnectabilityInCatalog(t *testing.T) {
	service := NewService(NewCatalog(nil), &serviceStore{})
	plugin, ok, err := service.Plugin(context.Background(), whatsapp.ProviderID, false)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Implementation.Status != "available" {
		t.Fatalf("implementation = %#v", plugin.Implementation)
	}
}

func TestServiceListsChatSessionPluginsInCatalog(t *testing.T) {
	service := NewService(NewCatalog(nil), &serviceStore{})
	plugins, err := service.ListPlugins(context.Background(), false)
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

func TestServiceDisconnectAccount(t *testing.T) {
	store := serviceStore{
		connections: []integrations.Connection{{
			ID:       "gmail:personal",
			Provider: gmailconnector.ProviderID,
			Alias:    "personal",
		}},
	}
	disconnecter := &fakeSessionDisconnecter{provider: gmailconnector.ProviderID}
	service := NewService(NewCatalog(nil), &store, disconnecter)
	result, err := service.DisconnectAccount(context.Background(), " gmail:personal ")
	if err != nil {
		t.Fatal(err)
	}
	if result.MCPServersChanged {
		t.Fatalf("result = %#v, want no MCP server change", result)
	}
	if len(store.connections) != 0 {
		t.Fatalf("connections = %#v", store.connections)
	}
	if disconnecter.connection.ID != "gmail:personal" {
		t.Fatalf("disconnecter connection = %#v", disconnecter.connection)
	}
	if _, err := service.DisconnectAccount(context.Background(), "gmail:missing"); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceDisconnectConnectionBackedMCPRefreshesTools(t *testing.T) {
	store := serviceStore{
		connections: []integrations.Connection{{
			ID:       "slack:acme-u1",
			Provider: slackconnector.ProviderID,
			Alias:    "acme",
		}},
	}
	service := NewService(NewCatalog(nil), &store)

	result, err := service.DisconnectAccount(context.Background(), "slack:acme-u1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.MCPServersChanged {
		t.Fatalf("result = %#v, want MCP server change", result)
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
	service := NewService(NewCatalog(nil), &store, disconnecter)

	if _, err := service.DisconnectAccount(ctx, "telegram:personal"); err != nil {
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
