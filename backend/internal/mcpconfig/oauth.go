package mcpconfig

import "cmp"

const OAuthCallbackPath = "/v1/mcp/oauth/callback"

func (c OAuthConfig) StaticClientID() string {
	return cmp.Or(c.ClientID, c.PublicClientID)
}

func (c OAuthConfig) UsesLoopback() bool {
	return c.ClientID == "" && c.PublicClientID != ""
}

type AuthorizationResponse struct {
	Code   string
	State  string
	Issuer string
	Error  string
}

func OAuthConnectionID(serverID string) string {
	return "mcp:" + serverID
}

func (s Server) TokenID() string {
	if s.TokenConnectionID != "" {
		return s.TokenConnectionID
	}
	return OAuthConnectionID(s.ID)
}
