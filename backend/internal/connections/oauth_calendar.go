package connections

import (
	"context"
	"fmt"
	"net/http"

	calendarconnector "github.com/wins/jaz/backend/internal/connectors/calendar"
	googleconnector "github.com/wins/jaz/backend/internal/connectors/google"
)

func newCalendarOAuthProvider(config googleconnector.OAuthClientConfig) oauthProvider {
	return googleOAuthProvider{
		providerID: calendarconnector.ProviderID,
		config:     config,
		scopes:     calendarconnector.OAuthScopes,
		verify: func(ctx context.Context, client *http.Client) (oauthIdentity, error) {
			info, err := calendarconnector.APIClient{HTTP: client}.UserInfo(ctx)
			if err != nil {
				return oauthIdentity{}, fmt.Errorf("google calendar verification failed: %w", err)
			}
			return oauthIdentity{
				accountID:   info.Email,
				accountName: info.Name,
			}, nil
		},
	}
}
