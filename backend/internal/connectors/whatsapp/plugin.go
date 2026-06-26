package whatsapp

import (
	"github.com/wins/jaz/backend/internal/connectors/chat"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	ProviderID   = "whatsapp"
	ProviderName = "WhatsApp"
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Sync WhatsApp conversations into chat memory and let agents send approved messages.",
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "chat",
		Icon: integrations.PluginIcon{
			Kind:       integrations.PluginIconKindInitials,
			Value:      "WA",
			Background: "#e7f7ee",
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindSession,
			Description: "Scan a WhatsApp QR code to link this Jaz instance as a companion device.",
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilitySync,
			integrations.CapabilityAct,
			integrations.CapabilityMaterialize,
		},
		MultiAccount: true,
		SourceLanes:  []string{"sources/chat/whatsapp"},
		Tools:        chatTools(),
		Skills:       chatSkills(),
		ConnectionNotes: []string{
			"Jaz stores raw WhatsApp contacts and messages under ~/.memory/raw-sources and writes readable chat pages under memory sources/chat.",
			"QR pairing requires a WhatsApp provider session adapter; the catalog entry is present before pairing is enabled.",
			"Message sending should go through approval and audit policies.",
			"Initial history depends on what WhatsApp Web makes available during companion-device sync.",
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

func chatSkills() []integrations.PluginSkill {
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
			ID:          "whatsapp",
			Name:        "WhatsApp",
			Description: "Provider-specific WhatsApp session, sync, and action caveats.",
			Status:      "planned",
		},
	}
}
