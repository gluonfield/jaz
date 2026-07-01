package connections

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type gmailOAuthProvider struct {
	config gmailconnector.OAuthClientConfig
}

func (p gmailOAuthProvider) id() string { return gmailconnector.ProviderID }

func (p gmailOAuthProvider) authCodeURL(redirectURL, state, verifier string) (string, error) {
	credentials, err := p.config.Credentials()
	if err != nil {
		return "", err
	}
	return gmailOAuthConfig(redirectURL, credentials).AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
	), nil
}

func (p gmailOAuthProvider) exchange(ctx context.Context, _ *http.Client, redirectURL, code, verifier string) (integrationoauth.Token, oauthIdentity, error) {
	credentials, err := p.config.Credentials()
	if err != nil {
		return integrationoauth.Token{}, oauthIdentity{}, err
	}
	token, err := gmailOAuthConfig(redirectURL, credentials).Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return integrationoauth.Token{}, oauthIdentity{}, fmt.Errorf("token exchange: %w", err)
	}
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	profile, err := gmailconnector.APIClient{HTTP: client}.Profile(ctx)
	if err != nil {
		return integrationoauth.Token{}, oauthIdentity{}, fmt.Errorf("gmail verification failed: %w", err)
	}
	accountID := strings.TrimSpace(profile.EmailAddress)
	stored := integrationoauth.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		AuthURL:      gmailconnector.OAuthAuthURL,
		TokenURL:     gmailconnector.OAuthTokenURL,
		AuthStyle:    int(oauth2.AuthStyleInParams),
		RedirectURL:  redirectURL,
		Scopes:       gmailconnector.OAuthScopes,
	}
	return stored, oauthIdentity{
		accountID:   accountID,
		accountName: accountID,
		scopes:      gmailconnector.OAuthScopes,
	}, nil
}

func gmailOAuthConfig(redirectURL string, credentials gmailconnector.OAuthClientCredentials) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   gmailconnector.OAuthAuthURL,
			TokenURL:  gmailconnector.OAuthTokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURL,
		Scopes:      gmailconnector.OAuthScopes,
	}
}
