package connections

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	googleconnector "github.com/wins/jaz/backend/internal/connectors/google"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type googleOAuthProvider struct {
	providerID string
	config     googleconnector.OAuthClientConfig
	scopes     []string
	verify     func(context.Context, *http.Client) (oauthIdentity, error)
}

func (p googleOAuthProvider) id() string { return p.providerID }

func (p googleOAuthProvider) authCodeURL(redirectURL, state, verifier string) (string, error) {
	credentials, err := p.config.Credentials()
	if err != nil {
		return "", err
	}
	return googleOAuthConfig(redirectURL, credentials, p.scopes).AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
	), nil
}

func (p googleOAuthProvider) exchange(ctx context.Context, httpClient *http.Client, redirectURL, code, verifier string) (integrationoauth.Token, oauthIdentity, error) {
	credentials, err := p.config.Credentials()
	if err != nil {
		return integrationoauth.Token{}, oauthIdentity{}, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	token, err := googleOAuthConfig(redirectURL, credentials, p.scopes).Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return integrationoauth.Token{}, oauthIdentity{}, fmt.Errorf("token exchange: %w", err)
	}
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	identity, err := p.verify(ctx, client)
	if err != nil {
		return integrationoauth.Token{}, oauthIdentity{}, err
	}
	identity.accountID = strings.TrimSpace(identity.accountID)
	identity.accountName = strings.TrimSpace(identity.accountName)
	if identity.accountName == "" {
		identity.accountName = identity.accountID
	}
	identity.scopes = p.scopes
	stored := integrationoauth.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		AuthURL:      googleconnector.OAuthAuthURL,
		TokenURL:     googleconnector.OAuthTokenURL,
		AuthStyle:    int(oauth2.AuthStyleInParams),
		RedirectURL:  redirectURL,
		Scopes:       p.scopes,
	}
	return stored, identity, nil
}

func googleOAuthConfig(redirectURL string, credentials googleconnector.OAuthClientCredentials, scopes []string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   googleconnector.OAuthAuthURL,
			TokenURL:  googleconnector.OAuthTokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURL,
		Scopes:      scopes,
	}
}
