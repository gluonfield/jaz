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
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          "gmail",
		Name:        "Gmail",
		Description: "Sync Gmail into memory and let agents read, draft, send, label, archive, and organize email.",
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
			Description: "Jaz-managed Google OAuth for sync and actions.",
			Scopes:      []string{ScopeReadonly, ScopeModify, ScopeCompose, ScopeSend},
		}, {
			Kind:        integrations.AuthKindRemoteMCP,
			Description: "Official Google Gmail MCP server compatibility path.",
			Scopes:      []string{ScopeReadonly, ScopeCompose},
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilitySync,
			integrations.CapabilityAct,
			integrations.CapabilityMaterialize,
			integrations.CapabilityMCP,
		},
		MultiAccount: true,
		SourceLanes:  []string{"sources/email"},
		Tools:        tools(),
		Skills: []integrations.PluginSkill{{
			ID:          "gmail",
			Name:        "Gmail",
			Description: "General guidance for reading, searching, drafting, sending, and organizing Gmail.",
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
			"Jaz supports multiple Gmail accounts through connection aliases such as personal or work.",
			"Jaz-owned Gmail tools use Google APIs directly so desktop builds do not need to ship OAuth client secrets.",
			"The official Gmail MCP endpoint is useful as a compatibility target, but is not the consumer-clean Jaz default.",
		},
		Implementation: integrations.Implementation{
			Status: "planned",
			Owner:  "jaz",
		},
	}
}

func tools() []integrations.PluginTool {
	return []integrations.PluginTool{
		readTool("get_profile", "Return the current Gmail user's profile information."),
		readTool("get_recent_emails", "Return the most recently received Gmail messages."),
		readTool("list_labels", "List Gmail labels with per-label totals for inbox, unread, and label count questions."),
		readTool("list_drafts", "List Gmail drafts with summarized metadata for review or selection."),
		readTool("search_emails", "Search Gmail messages using Gmail query operators and optional exact label IDs."),
		readTool("search_email_ids", "Retrieve Gmail message IDs matching a Gmail search query."),
		readTool("search_thread_ids", "Retrieve Gmail thread IDs matching a Gmail search query."),
		readTool("read_email", "Fetch a single Gmail message including body, metadata, labels, timestamp, and attachments."),
		readTool("batch_read_email", "Read multiple Gmail messages in one call."),
		readTool("read_email_thread", "Fetch an entire Gmail conversation thread from a message ID or thread ID."),
		readTool("batch_read_email_threads", "Fetch multiple Gmail conversation threads, resolving message IDs to thread IDs when needed."),
		readTool("batch_read_thread", "Fetch multiple known Gmail threads without resolving message IDs first."),
		readTool("read_attachment", "Read one attachment from a Gmail message using the parent message ID and attachment ID or exact filename."),
		tool("create_draft", "Create a Gmail draft without sending so the user can review it in Gmail.", integrations.ActionRiskDraft, ScopeCompose),
		tool("update_draft", "Update an existing Gmail draft in place while preserving omitted fields.", integrations.ActionRiskDraft, ScopeCompose),
		tool("send_draft", "Send an existing Gmail draft after explicit user review or instruction.", integrations.ActionRiskWrite, ScopeCompose),
		tool("send_email", "Send an email from the authenticated Gmail account.", integrations.ActionRiskWrite, ScopeSend),
		tool("forward_emails", "Forward Gmail messages with an optional note and preserved original attachments.", integrations.ActionRiskWrite, ScopeReadonly, ScopeSend),
		tool("create_label", "Create a Gmail label, returning the existing label when it already exists.", integrations.ActionRiskWrite, ScopeModify),
		tool("apply_labels_to_emails", "Apply labels to Gmail messages by label name rather than Gmail label ID.", integrations.ActionRiskWrite, ScopeModify),
		tool("batch_modify_email", "Add or remove Gmail labels on a batch of individual messages.", integrations.ActionRiskWrite, ScopeModify),
		tool("bulk_label_matching_emails", "Apply a label to every Gmail message matching a Gmail search query without sending IDs through model context.", integrations.ActionRiskBulkWrite, ScopeModify),
		tool("archive_emails", "Archive Gmail messages by removing INBOX while keeping them searchable in Gmail.", integrations.ActionRiskWrite, ScopeModify),
		tool("delete_emails", "Move Gmail messages to Trash without permanently deleting them.", integrations.ActionRiskDelete, ScopeModify),
	}
}

func readTool(name, description string) integrations.PluginTool {
	return tool(name, description, integrations.ActionRiskRead, ScopeReadonly)
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
