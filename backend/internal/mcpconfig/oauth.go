package mcpconfig

const OAuthCallbackPath = "/v1/mcp/oauth/callback"

func OAuthConnectionID(serverID string) string {
	return "mcp:" + serverID
}
