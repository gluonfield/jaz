package telegram

import (
	"github.com/wins/jaz/backend/internal/connectors/chat"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	ProviderID   = "telegram"
	ProviderName = "Telegram"

	ToolSearch                 = "telegram_search"
	ToolSearchDescription      = "Search Telegram people and chats from a connected account. Returns recipient values usable with telegram_send_message."
	ToolSendMessage            = "telegram_send_message"
	ToolSendMessageDescription = "Send a Telegram message from one connected account to a username, user id, or chat id. Requires a connected Telegram session."
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Sync Telegram chats into raw chat archives and let agents send messages.",
		Examples: []string{
			"Summarize my recent Telegram chats",
			"Catch me up on unread group chats",
			"Send a Telegram message to a contact",
		},
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "chat",
		Icon: integrations.PluginIcon{
			Kind:  integrations.PluginIconKindAsset,
			Value: ProviderID,
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindSession,
			Description: "Scan a Telegram login QR code to connect this Jaz instance.",
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilitySync,
			integrations.CapabilityAct,
		},
		MultiAccount: true,
		Tools:        chatTools(),
		Skills:       telegramSkills(),
		ConnectionNotes: []string{
			"Jaz stores raw Telegram contacts and messages under the configured Jaz ingest root.",
			"Message sends are direct actions from the selected connected account.",
		},
		Implementation: integrations.Implementation{
			Status: "available",
			Owner:  "jaz",
		},
	}
}

func chatTools() []integrations.PluginTool {
	return []integrations.PluginTool{
		{
			Name:        ToolSearch,
			Description: ToolSearchDescription,
			Capability:  integrations.CapabilityAct,
			Risk:        integrations.ActionRiskRead,
		},
		{
			Name:        ToolSendMessage,
			Description: ToolSendMessageDescription,
			Capability:  integrations.CapabilityAct,
			Risk:        integrations.ActionRiskWrite,
		},
	}
}

func telegramSkills() []integrations.PluginSkill {
	return []integrations.PluginSkill{
		{
			ID:          chat.SkillChatMemory,
			Name:        "Chat Memory",
			Description: "Guidance for reading unified chat source pages and raw chat archives.",
			Status:      "planned",
		},
		{
			ID:          chat.SkillChatActions,
			Name:        "Chat Actions",
			Description: "Guidance for safe chat sends, replies, reactions, and provider caveats.",
			Status:      "planned",
		},
		{
			ID:          "telegram",
			Name:        "Telegram",
			Description: "Provider-specific Telegram session, sync, and action caveats.",
			Status:      "planned",
		},
	}
}
