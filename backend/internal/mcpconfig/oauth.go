package mcpconfig

import "strings"

func OAuthConnectionID(serverID string) string {
	return "mcp:" + strings.TrimSpace(serverID)
}
