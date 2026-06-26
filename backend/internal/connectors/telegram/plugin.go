package telegram

import (
	"github.com/wins/jaz/backend/internal/connectors/chat"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	ProviderID   = "telegram"
	ProviderName = "Telegram"
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Sync Telegram chats into chat memory and let agents send approved messages.",
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "chat",
		Icon: integrations.PluginIcon{
			Kind:       integrations.PluginIconKindInitials,
			Value:      "TG",
			Background: "#e8f3ff",
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindSession,
			Description: "Scan a Telegram login QR code to connect this Jaz instance.",
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilitySync,
			integrations.CapabilityAct,
			integrations.CapabilityMaterialize,
		},
		MultiAccount: true,
		SourceLanes:  []string{"sources/chat/telegram"},
		Tools:        chatTools(),
		Skills:       telegramSkills(),
		ConnectionNotes: []string{
			"Jaz stores raw Telegram contacts and messages under ~/.memory/raw-sources and writes readable chat pages under memory sources/chat.",
			"Telegram QR login requires a Telegram client app identity in the provider adapter.",
			"The catalog entry is present before QR pairing is enabled.",
			"Message sending should go through approval and audit policies.",
		},
		Implementation: integrations.Implementation{
			Status: "planned",
			Owner:  "jaz",
		},
	}
}

func chatTools() []integrations.PluginTool {
	return []integrations.PluginTool{
		{
			Name:        chat.ToolSendMessage,
			Description: "Send a message to a connected chat conversation after approval.",
			Capability:  integrations.CapabilityAct,
			Risk:        integrations.ActionRiskWrite,
		},
		{
			Name:        chat.ToolReply,
			Description: "Reply to a specific chat message after approval.",
			Capability:  integrations.CapabilityAct,
			Risk:        integrations.ActionRiskWrite,
		},
		{
			Name:        chat.ToolAddReaction,
			Description: "Add a reaction to a chat message.",
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
			Description: "Guidance for safe chat sends, replies, reactions, approvals, and audit trails.",
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
