package connections

import (
	"context"
	"fmt"
	"net/http"

	slackconnector "github.com/wins/jaz/backend/internal/connectors/slack"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type slackOAuthProvider struct {
	config slackconnector.OAuthClientConfig
}

func (p slackOAuthProvider) id() string { return slackconnector.ProviderID }

func (p slackOAuthProvider) authCodeURL(redirectURL, state, verifier string) (string, error) {
	return slackconnector.AuthorizeURL(p.config.Resolve(), redirectURL, state, verifier), nil
}

func (p slackOAuthProvider) exchange(ctx context.Context, httpClient *http.Client, redirectURL, code, verifier string) (integrationoauth.Token, oauthIdentity, error) {
	clientID := p.config.Resolve()
	accessToken, err := slackconnector.Exchange(ctx, httpClient, clientID, redirectURL, code, verifier)
	if err != nil {
		return integrationoauth.Token{}, oauthIdentity{}, err
	}
	profile, err := slackconnector.Identify(ctx, httpClient, accessToken)
	if err != nil {
		return integrationoauth.Token{}, oauthIdentity{}, fmt.Errorf("slack verification failed: %w", err)
	}
	stored := integrationoauth.Token{AccessToken: accessToken}
	return stored, oauthIdentity{
		accountID:   profile.AccountID(),
		accountName: profile.AccountName(),
		scopes:      slackconnector.UserScopes,
	}, nil
}
