package connections

import (
	"context"
	"testing"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestServiceReportsGmailConnectionState(t *testing.T) {
	service := NewService(NewCatalog(), serviceStore{})
	plugin, ok, err := service.Plugin(context.Background(), gmailconnector.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusNotConnected || len(plugin.Connection.Accounts) != 0 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}

	service = NewService(NewCatalog(), serviceStore{
		tokens: map[string]integrationoauth.Token{
			gmailconnector.OAuthConnectionID: {
				RefreshToken: "refresh",
				Scopes:       []string{gmailconnector.ScopeModify},
			},
		},
	})
	plugin, ok, err = service.Plugin(context.Background(), gmailconnector.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusConnected || len(plugin.Connection.Accounts) != 1 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
	account := plugin.Connection.Accounts[0]
	if account.ID != gmailconnector.OAuthConnectionID || account.Provider != gmailconnector.ProviderID || account.Alias != "default" || len(account.Scopes) != 1 || account.Scopes[0] != gmailconnector.ScopeModify {
		t.Fatalf("account = %#v", account)
	}
}

func TestServiceReturnsSavedGmailAccounts(t *testing.T) {
	service := NewService(NewCatalog(), serviceStore{
		connections: []integrations.Connection{{
			ID:          gmailconnector.OAuthConnectionID,
			Provider:    gmailconnector.ProviderID,
			AccountID:   "augustinas@example.com",
			AccountName: "Augustinas",
			Alias:       "personal",
			Scopes:      []string{gmailconnector.ScopeModify},
		}},
	})
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

type serviceStore struct {
	tokens      map[string]integrationoauth.Token
	connections []integrations.Connection
}

func (s serviceStore) LoadToken(_ context.Context, connectionID string) (integrationoauth.Token, bool, error) {
	token, ok := s.tokens[connectionID]
	return token, ok, nil
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
