package mcpconfig

import "time"

// OAuthToken is a persisted OAuth credential for an MCP server, together with the
// resolved client and endpoints needed to refresh it later.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`

	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	AuthURL      string   `json:"auth_url"`
	TokenURL     string   `json:"token_url"`
	AuthStyle    int      `json:"auth_style"`
	RedirectURL  string   `json:"redirect_url,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	Resource     string   `json:"resource,omitempty"`
}

// OAuthTokenStore persists OAuth tokens for MCP servers.
type OAuthTokenStore interface {
	LoadMCPOAuthToken(serverID string) (OAuthToken, bool, error)
	SaveMCPOAuthToken(serverID string, token OAuthToken) error
	DeleteMCPOAuthToken(serverID string) error
}
