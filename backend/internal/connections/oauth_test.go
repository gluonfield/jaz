package connections

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"

	calendarconnector "github.com/wins/jaz/backend/internal/connectors/calendar"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	googleconnector "github.com/wins/jaz/backend/internal/connectors/google"
	slackconnector "github.com/wins/jaz/backend/internal/connectors/slack"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type memoryOAuthStore struct {
	connections []integrations.Connection
}

var testOAuthConfig = OAuthConfig{
	Calendar: googleconnector.OAuthClientConfig{
		ClientID:     "test-calendar-client",
		ClientSecret: "test-calendar-secret",
	},
	Gmail: googleconnector.OAuthClientConfig{
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

func TestOAuthStartBuildsGoogleCalendarPKCEURL(t *testing.T) {
	redirectURL := "http://127.0.0.1:5222/v1/connections/oauth/callback"
	start, err := NewOAuthService(memoryOAuthStore{}, testOAuthConfig).Start(context.Background(), calendarconnector.ProviderID, redirectURL)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(start.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	scope := strings.Fields(q.Get("scope"))
	if parsed.Host != "accounts.google.com" || q.Get("client_id") != "test-calendar-client" || q.Get("redirect_uri") != redirectURL {
		t.Fatalf("auth url = %s", start.AuthURL)
	}
	for _, want := range []string{calendarconnector.ScopeEvents, calendarconnector.ScopeUserInfoEmail} {
		if !slices.Contains(scope, want) {
			t.Fatalf("scope %q missing from %#v", want, scope)
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

func TestOAuthStartBuildsGmailPKCEURL(t *testing.T) {
	redirectURL := "http://127.0.0.1:5222/v1/connections/oauth/callback"
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

func TestOAuthStartUsesBundledGmailClientCredentials(t *testing.T) {
	redirectURL := "http://127.0.0.1:5222/v1/connections/oauth/callback"
	start, err := NewOAuthService(memoryOAuthStore{}, OAuthConfig{}).Start(context.Background(), gmailconnector.ProviderID, redirectURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(start.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("client_id") != googleconnector.OAuthClientID {
		t.Fatalf("auth url = %s", start.AuthURL)
	}
}

func TestOAuthStartRejectsUnsupportedPlugin(t *testing.T) {
	_, err := NewOAuthService(memoryOAuthStore{}, OAuthConfig{}).Start(context.Background(), "notion", "http://127.0.0.1/callback")
	if err == nil {
		t.Fatal("expected unsupported plugin error")
	}
}

func TestOAuthStartBuildsSlackPKCEURL(t *testing.T) {
	redirectURL := "http://127.0.0.1:5222/v1/connections/oauth/callback"
	start, err := NewOAuthService(memoryOAuthStore{}, OAuthConfig{}).Start(context.Background(), slackconnector.ProviderID, redirectURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(start.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if parsed.Host != "slack.com" ||
		q.Get("client_id") != slackconnector.OAuthClientID ||
		q.Get("redirect_uri") != redirectURL ||
		q.Get("response_type") != "code" ||
		q.Get("code_challenge_method") != "S256" ||
		q.Get("code_challenge") == "" ||
		q.Get("state") == "" {
		t.Fatalf("slack auth url = %s", start.AuthURL)
	}
	scope := strings.Split(q.Get("scope"), ",")
	if !slices.Contains(scope, "search:read.public") || !slices.Contains(scope, "chat:write") {
		t.Fatalf("slack scopes missing from %#v", scope)
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

	connection, err := service.upsertConnection(context.Background(), gmailconnector.ProviderID, gmailIdentity("Augustinas@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if connection.ID != gmailconnector.OAuthConnectionID || connection.Alias != "default" || connection.AccountID != "Augustinas@example.com" {
		t.Fatalf("connection = %#v", connection)
	}
}

func gmailIdentity(email string) oauthIdentity {
	return oauthIdentity{
		accountID:   email,
		accountName: email,
		scopes:      gmailconnector.OAuthScopes,
	}
}

func TestSlackConnectionAliasFromWorkspaceName(t *testing.T) {
	service := NewOAuthService(memoryOAuthStore{}, OAuthConfig{})

	connection, err := service.upsertConnection(context.Background(), slackconnector.ProviderID, oauthIdentity{
		accountID:   "T1-U1",
		accountName: "Acme / augustinas",
		scopes:      slackconnector.UserScopes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if connection.Alias != "acme-augustinas" || connection.AccountID != "T1-U1" || connection.Provider != slackconnector.ProviderID {
		t.Fatalf("connection = %#v", connection)
	}
}

func TestGmailConnectionCreatesAccountSpecificID(t *testing.T) {
	service := NewOAuthService(memoryOAuthStore{connections: []integrations.Connection{{
		ID:        gmailconnector.OAuthConnectionID,
		Provider:  gmailconnector.ProviderID,
		AccountID: "first@example.com",
	}}}, OAuthConfig{})

	connection, err := service.upsertConnection(context.Background(), gmailconnector.ProviderID, gmailIdentity("second@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if connection.ID == gmailconnector.OAuthConnectionID || connection.ID == "" || connection.Alias != "second" || connection.Provider != gmailconnector.ProviderID {
		t.Fatalf("connection = %#v", connection)
	}
}

func TestCalendarConnectionCreatesAccountSpecificID(t *testing.T) {
	service := NewOAuthService(memoryOAuthStore{}, OAuthConfig{})

	connection, err := service.upsertConnection(context.Background(), calendarconnector.ProviderID, oauthIdentity{
		accountID:   "augustinas@example.com",
		accountName: "Augustinas",
		scopes:      calendarconnector.OAuthScopes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if connection.ID == "" || connection.Provider != calendarconnector.ProviderID || connection.AccountID != "augustinas@example.com" || connection.Alias != "augustinas" {
		t.Fatalf("connection = %#v", connection)
	}
}
