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
	ToolSearchThreads  = "gmail_search_threads"
	ToolReadThread     = "gmail_read_thread"
	ToolCreateDraft    = "gmail_create_draft"
	ToolCreateReply    = "gmail_create_reply_draft"
	ToolSendDraft      = "gmail_send_draft"
	ToolUpdateDraft    = "gmail_update_draft"
	ToolListDrafts     = "gmail_list_drafts"
	ToolReadAttachment = "gmail_read_attachment"
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          "gmail",
		Name:        "Gmail",
		Description: "Search Gmail threads, draft replies, and send approved drafts from connected accounts.",
		Examples: []string{
			"Summarize my unread inbox and flag what needs a reply",
			"Draft a reply to my latest thread",
			"Find recent emails about invoices",
		},
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
			Description: "Guidance for reading threads, drafting replies, and sending approved Gmail drafts.",
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
		tool(ToolSearchThreads, "Search Gmail conversation threads and return thread IDs with summarized message metadata.", integrations.ActionRiskRead, ScopeModify),
		tool(ToolReadThread, "Read a Gmail conversation thread by message ID or thread ID with bounded message bodies.", integrations.ActionRiskRead, ScopeModify),
		tool(ToolCreateDraft, "Create a new plain text Gmail draft to specified recipients.", integrations.ActionRiskWrite, ScopeModify),
		tool(ToolCreateReply, "Create a reply or reply-all draft for an existing Gmail message or thread.", integrations.ActionRiskWrite, ScopeModify),
		tool(ToolSendDraft, "Send an existing Gmail draft after review or explicit approval.", integrations.ActionRiskWrite, ScopeModify),
		tool(ToolUpdateDraft, "Update an existing Gmail draft in place while preserving omitted fields.", integrations.ActionRiskWrite, ScopeModify),
		tool(ToolListDrafts, "List Gmail drafts with summarized message metadata.", integrations.ActionRiskRead, ScopeModify),
		tool(ToolReadAttachment, "Explicitly fetch a Gmail attachment by message ID and attachment ID and return its local file path.", integrations.ActionRiskRead, ScopeModify),
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
