package granola

import "github.com/wins/jaz/backend/pkg/integrations"

const ProviderID = "granola"

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        "Granola",
		Description: "Search meeting notes and transcripts, and organise them into folders, with Granola's official MCP server.",
		Examples: []string{
			"Summarise what we agreed in yesterday's meetings",
			"Find the transcript of my last customer call",
			"List open action items from this week's meetings",
		},
		Provider:       integrations.Provider{ID: ProviderID, Name: "Granola"},
		Category:       "meetings",
		Icon:           integrations.PluginIcon{Kind: integrations.PluginIconKindAsset, Value: ProviderID},
		Auth:           []integrations.AuthOption{{Kind: integrations.AuthKindMCPConnection}},
		Capabilities:   []integrations.Capability{integrations.CapabilityAct, integrations.CapabilityMCP},
		RemoteMCP:      &integrations.RemoteMCP{URL: "https://mcp.granola.ai/mcp", Status: "available"},
		Implementation: integrations.Implementation{Status: "available", Owner: "granola"},
	}
}
