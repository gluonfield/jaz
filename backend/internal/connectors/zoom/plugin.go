package zoom

import "github.com/wins/jaz/backend/pkg/integrations"

const ProviderID = "zoom"

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:             ProviderID,
		Name:           "Zoom",
		Description:    "Search meeting recordings, transcripts and notes with Zoom’s official MCP server.",
		Examples:       []string{"Find the recording of my last customer meeting", "Summarise the action items from yesterday’s Zoom calls"},
		Provider:       integrations.Provider{ID: ProviderID, Name: "Zoom"},
		Category:       "meetings",
		Icon:           integrations.PluginIcon{Kind: integrations.PluginIconKindAsset, Value: ProviderID},
		Auth:           []integrations.AuthOption{{Kind: integrations.AuthKindMCPConnection}},
		Capabilities:   []integrations.Capability{integrations.CapabilityAct, integrations.CapabilityMCP},
		RemoteMCP:      &integrations.RemoteMCP{URL: "https://mcp.zoom.us/mcp/zoom/streamable", Status: "available", OAuthSecrets: true},
		Implementation: integrations.Implementation{Status: "available", Owner: ProviderID},
	}
}
