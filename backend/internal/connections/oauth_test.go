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

type memoryOAuthStore struct{}

func (memoryOAuthStore) SaveOAuthConnection(context.Context, integrationoauth.Token, integrations.Connection) error {
	return nil
}

func TestOAuthStartBuildsGmailPKCEURL(t *testing.T) {
	redirectURL := "http://127.0.0.1:5222/v1/connections/oauth/google/callback"
	start, err := NewOAuthService(memoryOAuthStore{}, OAuthConfig{}).Start(context.Background(), gmailconnector.ProviderID, redirectURL)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(start.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	scope := strings.Fields(q.Get("scope"))
	if parsed.Host != "accounts.google.com" || q.Get("client_id") != gmailconnector.OAuthClientID || q.Get("redirect_uri") != redirectURL {
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
