package connections

import (
	"context"
	"testing"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestServiceReportsGmailConnectionState(t *testing.T) {
	service := NewService(NewCatalog(), serviceTokenStore{})
	plugin, ok, err := service.Plugin(context.Background(), gmailconnector.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusNotConnected || len(plugin.Connection.Accounts) != 0 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}

	service = NewService(NewCatalog(), serviceTokenStore{
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

type serviceTokenStore struct {
	tokens map[string]integrationoauth.Token
}

func (s serviceTokenStore) LoadToken(_ context.Context, connectionID string) (integrationoauth.Token, bool, error) {
	token, ok := s.tokens[connectionID]
	return token, ok, nil
}

func (s serviceTokenStore) SaveToken(context.Context, string, integrationoauth.Token) error {
	return nil
}
