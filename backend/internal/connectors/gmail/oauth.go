package gmail

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	OAuthClientID = "181926640908-oms6rae8dt4bcdsbvgfqts0b1b9tvhkr.apps.googleusercontent.com"
	// Google desktop OAuth clients cannot keep this confidential; Google documents
	// embedding it in installed-app source, where it is not treated as a secret.
	OAuthClientSecret = "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"
	OAuthAuthURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	OAuthTokenURL     = "https://oauth2.googleapis.com/token"
	// OAuthConnectionID is the legacy single-account connection ID. New Gmail
	// connections use ConnectionID(accountID), but existing rows keep working.
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

func ConnectionID(accountID string) (string, error) {
	accountID = strings.ToLower(strings.TrimSpace(accountID))
	alias := integrations.NormalizeAlias(accountID)
	if alias == "" {
		return "", errors.New("gmail account id is required")
	}
	sum := sha256.Sum256([]byte(accountID))
	return fmt.Sprintf("%s:%s-%x", ProviderID, alias, sum[:4]), nil
}
