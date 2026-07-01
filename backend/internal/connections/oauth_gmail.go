package connections

import (
	"context"
	"fmt"
	"net/http"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	googleconnector "github.com/wins/jaz/backend/internal/connectors/google"
)

func newGmailOAuthProvider(config googleconnector.OAuthClientConfig) oauthProvider {
	return googleOAuthProvider{
		providerID: gmailconnector.ProviderID,
		config:     config,
		scopes:     gmailconnector.OAuthScopes,
		verify: func(ctx context.Context, client *http.Client) (oauthIdentity, error) {
			profile, err := gmailconnector.APIClient{HTTP: client}.Profile(ctx)
			if err != nil {
				return oauthIdentity{}, fmt.Errorf("gmail verification failed: %w", err)
			}
			return oauthIdentity{
				accountID:   profile.EmailAddress,
				accountName: profile.EmailAddress,
			}, nil
		},
	}
}
