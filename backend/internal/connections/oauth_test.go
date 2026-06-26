package connections

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type memoryOAuthStore struct {
	connections []integrations.Connection
}

var testOAuthConfig = OAuthConfig{
	Gmail: gmailconnector.OAuthClientConfig{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
	},
}

func (s memoryOAuthStore) ListConnections(_ context.Context, provider string) ([]integrations.Connection, error) {
	var out []integrations.Connection
	for _, connection := range s.connections {
		if connection.Provider == provider {
			out = append(out, connection)
		}
	}
	return out, nil
}

func (memoryOAuthStore) SaveOAuthConnection(context.Context, integrationoauth.Token, integrations.Connection) error {
	return nil
}

func TestOAuthStartBuildsGmailPKCEURL(t *testing.T) {
	redirectURL := "http://127.0.0.1:5222/v1/connections/oauth/google/callback"
	start, err := NewOAuthService(memoryOAuthStore{}, testOAuthConfig).Start(context.Background(), gmailconnector.ProviderID, redirectURL)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(start.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	scope := strings.Fields(q.Get("scope"))
	if parsed.Host != "accounts.google.com" || q.Get("client_id") != "test-client" || q.Get("redirect_uri") != redirectURL {
		t.Fatalf("auth url = %s", start.AuthURL)
	}
	if !slices.Contains(scope, gmailconnector.ScopeModify) {
		t.Fatalf("modify scope missing from %#v", scope)
	}
	for _, unwanted := range []string{gmailconnector.ScopeReadonly, gmailconnector.ScopeCompose, gmailconnector.ScopeSend} {
		if slices.Contains(scope, unwanted) {
			t.Fatalf("scope %q should not be requested in %#v", unwanted, scope)
		}
	}
	if q.Get("response_type") != "code" ||
		q.Get("access_type") != "offline" ||
		q.Get("prompt") != "consent" ||
		q.Get("code_challenge_method") != "S256" ||
		q.Get("code_challenge") == "" ||
		q.Get("state") == "" {
		t.Fatalf("oauth query = %#v", q)
	}
}

func TestOAuthStartRejectsUnsupportedPlugin(t *testing.T) {
	_, err := NewOAuthService(memoryOAuthStore{}, OAuthConfig{}).Start(context.Background(), "slack", "http://127.0.0.1/callback")
	if err == nil {
		t.Fatal("expected unsupported plugin error")
	}
}

func TestGmailConnectionReusesExistingAccount(t *testing.T) {
	existing := integrations.Connection{
		ID:          gmailconnector.OAuthConnectionID,
		Provider:    gmailconnector.ProviderID,
		AccountID:   "augustinas@example.com",
		AccountName: "Old name",
		Alias:       "default",
	}
	service := NewOAuthService(memoryOAuthStore{connections: []integrations.Connection{existing}}, OAuthConfig{})

	connection, err := service.gmailConnection(context.Background(), gmailconnector.Profile{EmailAddress: "Augustinas@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if connection.ID != gmailconnector.OAuthConnectionID || connection.Alias != "default" || connection.AccountID != "Augustinas@example.com" {
		t.Fatalf("connection = %#v", connection)
	}
}

func TestGmailConnectionCreatesAccountSpecificID(t *testing.T) {
	service := NewOAuthService(memoryOAuthStore{connections: []integrations.Connection{{
		ID:        gmailconnector.OAuthConnectionID,
		Provider:  gmailconnector.ProviderID,
		AccountID: "first@example.com",
	}}}, OAuthConfig{})

	connection, err := service.gmailConnection(context.Background(), gmailconnector.Profile{EmailAddress: "second@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if connection.ID == gmailconnector.OAuthConnectionID || connection.ID == "" || connection.Alias != "second" || connection.Provider != gmailconnector.ProviderID {
		t.Fatalf("connection = %#v", connection)
	}
}
