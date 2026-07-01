package gmail

import (
	"errors"
	"strings"
)

const (
	OAuthClientID = "818853148911-thp5kku4b4uh4vs8omhuq528847gb48h.apps.googleusercontent.com"
	// Public desktop OAuth client used with authorization-code + PKCE. It
	// identifies Jaz; Gmail access still requires a per-account refresh token.
	OAuthClientSecret = "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"
	OAuthAuthURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	OAuthTokenURL     = "https://oauth2.googleapis.com/token"
	// OAuthConnectionID is the legacy single-account connection ID. New Gmail
	// connections use integrations.ConnectionID, but existing rows keep working.
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
