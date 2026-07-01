package slack

import "github.com/wins/jaz/backend/pkg/integrations"

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Search Slack messages, read channels and DMs, and post messages through Slack's official MCP server.",
		Examples: []string{
			"Summarize what I missed in #engineering today",
			"Find recent Slack messages about the launch",
			"Post a status update to #general",
		},
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "chat",
		Icon: integrations.PluginIcon{
			Kind:  integrations.PluginIconKindAsset,
			Value: "slack",
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindOAuth,
			Description: "Jaz-managed Slack OAuth (public client + PKCE) for the official Slack MCP server.",
			Scopes:      UserScopes,
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilityAct,
			integrations.CapabilityMCP,
		},
		MultiAccount: true,
		Tools:        tools(),
		RemoteMCP: &integrations.RemoteMCP{
			URL:       RemoteMCPURL,
			Status:    "available",
			Requires:  []string{"Slack user token"},
			TokenAuth: true,
		},
		ConnectionNotes: []string{
			"Connect each Slack workspace separately.",
			"Slack tools are served by Slack's official MCP server using your user token.",
			"Slack requires the backing app to be Marketplace-published or internal; unlisted apps cannot use MCP.",
		},
		Implementation: integrations.Implementation{
			Status: "available",
			Owner:  "jaz",
		},
	}
}

func tools() []integrations.PluginTool {
	return []integrations.PluginTool{
		tool("slack_search_messages", "Search Slack messages across channels and DMs the user can access.", integrations.ActionRiskRead),
		tool("slack_list_channels", "List Slack channels the user is a member of.", integrations.ActionRiskRead),
		tool("slack_read_channel", "Read recent messages from a Slack channel or conversation.", integrations.ActionRiskRead),
		tool("slack_list_users", "Look up Slack users in the workspace.", integrations.ActionRiskRead),
		tool("slack_post_message", "Post a message to a Slack channel or conversation after approval.", integrations.ActionRiskWrite),
	}
}

func tool(name, description string, risk integrations.ActionRisk) integrations.PluginTool {
	return integrations.PluginTool{
		Name:        name,
		Description: description,
		Capability:  integrations.CapabilityAct,
		Risk:        risk,
	}
}
