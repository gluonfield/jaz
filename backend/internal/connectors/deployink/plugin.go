package deployink

import "github.com/wins/jaz/backend/pkg/integrations"

const (
	ProviderID   = "deployink"
	ProviderName = "Deployink"
	RemoteMCPURL = "https://mcp.ml.ink"
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Deploy and manage Ink services, databases, DNS, logs, and project resources through Ink's MCP server.",
		Examples: []string{
			"Deploy this app to Ink",
			"Show my failing deployments and recent logs",
			"Create a Postgres database and wire it to my service",
		},
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "deployment",
		Icon: integrations.PluginIcon{
			Kind:  integrations.PluginIconKindAsset,
			Value: "ink",
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindRemoteMCP,
			Description: "Remote Streamable HTTP MCP server at https://mcp.ml.ink.",
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilityAct,
			integrations.CapabilityMCP,
		},
		RemoteMCP: &integrations.RemoteMCP{
			URL:          RemoteMCPURL,
			Status:       "available",
			OAuthSecrets: false,
		},
		ConnectionNotes: []string{
			"Jaz connects to https://mcp.ml.ink and forwards every tool the MCP server advertises.",
			"If the MCP server asks for authorization, complete sign-in from Settings > MCP servers.",
		},
		Implementation: integrations.Implementation{
			Status: "available",
			Owner:  "jaz",
		},
	}
}
