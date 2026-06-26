package gmail

import "github.com/wins/jaz/backend/pkg/integrations"

const (
	ProviderID   = "gmail"
	ProviderName = "Gmail"

	ScopeReadonly = "https://www.googleapis.com/auth/gmail.readonly"
	ScopeModify   = "https://www.googleapis.com/auth/gmail.modify"
	ScopeCompose  = "https://www.googleapis.com/auth/gmail.compose"
	ScopeSend     = "https://www.googleapis.com/auth/gmail.send"

	RemoteMCPURL = "https://gmailmcp.googleapis.com/mcp/v1"

	ToolGetProfile     = "gmail_get_profile"
	ToolSearchMessages = "gmail_search_messages"
	ToolReadMessage    = "gmail_read_message"
	ToolSendMessage    = "gmail_send_message"
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          "gmail",
		Name:        "Gmail",
		Description: "Search, read, and send Gmail messages from connected accounts.",
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "email",
		Icon: integrations.PluginIcon{
			Kind:  integrations.PluginIconKindAsset,
			Value: "gmail",
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindOAuth,
			Description: "Jaz-managed Google OAuth for Gmail tools.",
			Scopes:      OAuthScopes,
		}, {
			Kind:        integrations.AuthKindRemoteMCP,
			Description: "Official Google Gmail MCP server compatibility path.",
			Scopes:      []string{ScopeReadonly, ScopeCompose, ScopeSend},
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilityAct,
			integrations.CapabilityMCP,
		},
		MultiAccount: true,
		Tools:        tools(),
		Skills: []integrations.PluginSkill{{
			ID:          "gmail",
			Name:        "Gmail",
			Description: "Guidance for reading, searching, sending, and organizing Gmail.",
			Status:      "planned",
		}, {
			ID:          "gmail-inbox-triage",
			Name:        "Gmail Inbox Triage",
			Description: "Inbox review and prioritization workflow guidance for Gmail.",
			Status:      "planned",
		}},
		RemoteMCP: &integrations.RemoteMCP{
			URL:          RemoteMCPURL,
			Status:       "developer_preview",
			Requires:     []string{"Google Cloud OAuth client", "Gmail MCP API enabled", "OAuth client secret for third-party MCP clients"},
			OAuthSecrets: true,
		},
		ConnectionNotes: []string{
			"Connect each Gmail account separately.",
			"Jaz-owned Gmail tools use Google APIs directly and require a Gmail-enabled Google OAuth client.",
			"Custom builds can supply another Gmail-enabled OAuth client through Jaz configuration.",
			"The official Gmail MCP endpoint is useful as a compatibility target, but is not the consumer-clean Jaz default.",
		},
		Implementation: integrations.Implementation{
			Status: "available",
			Owner:  "jaz",
		},
	}
}

func tools() []integrations.PluginTool {
	return []integrations.PluginTool{
		tool(ToolGetProfile, "Show profile totals for one connected Gmail account.", integrations.ActionRiskRead, ScopeModify),
		tool(ToolSearchMessages, "Search Gmail messages and return bounded metadata, snippets, labels, and message IDs.", integrations.ActionRiskRead, ScopeModify),
		tool(ToolReadMessage, "Read one Gmail message by ID with headers, labels, body text or HTML, and attachment metadata.", integrations.ActionRiskRead, ScopeModify),
		tool(ToolSendMessage, "Send a plain text Gmail message from one connected account.", integrations.ActionRiskWrite, ScopeModify),
	}
}

func tool(name, description string, risk integrations.ActionRisk, scopes ...string) integrations.PluginTool {
	return integrations.PluginTool{
		Name:           name,
		Description:    description,
		Capability:     integrations.CapabilityAct,
		Risk:           risk,
		RequiredScopes: scopes,
	}
}
