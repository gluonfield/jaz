package gmail

import (
	"errors"
	"strings"
)

const (
	OAuthClientID = "jaz-google-oauth-client-id"
	// Google desktop OAuth clients cannot keep this confidential; Google documents
	// embedding it in installed-app source, where it is not treated as a secret.
	OAuthClientSecret = "jaz-google-oauth-client-secret"
	OAuthAuthURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	OAuthTokenURL     = "https://oauth2.googleapis.com/token"
	OAuthConnectionID = "gmail:default"
)

var OAuthScopes = []string{ScopeModify}

type OAuthClientConfig struct {
	ClientID     string
	ClientSecret string
}

type OAuthClientCredentials struct {
	ClientID     string
	ClientSecret string
}

func (c OAuthClientConfig) Credentials() (OAuthClientCredentials, error) {
	id := strings.TrimSpace(c.ClientID)
	secret := strings.TrimSpace(c.ClientSecret)
	if id == "" && secret == "" {
		return DefaultOAuthClientCredentials(), nil
	}
	if id == "" || secret == "" {
		return OAuthClientCredentials{}, errors.New("both Gmail OAuth client ID and secret are required when overriding the bundled client")
	}
	return OAuthClientCredentials{ClientID: id, ClientSecret: secret}, nil
}

func DefaultOAuthClientCredentials() OAuthClientCredentials {
	return OAuthClientCredentials{
		ClientID:     OAuthClientID,
		ClientSecret: OAuthClientSecret,
	}
}
