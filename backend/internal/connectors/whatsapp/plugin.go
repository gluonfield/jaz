package whatsapp

import (
	"github.com/wins/jaz/backend/internal/connectors/chat"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	ProviderID   = "whatsapp"
	ProviderName = "WhatsApp"

	ToolSearch                 = "whatsapp_search"
	ToolSearchDescription      = "Search WhatsApp people and chats from a connected account. Returns recipient values usable with whatsapp_send_message."
	ToolReadRecent             = "whatsapp_read_recent"
	ToolReadRecentDescription  = "Read recent WhatsApp messages from a chat JID or phone number from a connected account."
	ToolSendMessage            = "whatsapp_send_message"
	ToolSendMessageDescription = "Send a WhatsApp message from one connected account to a phone number or WhatsApp JID. Requires a connected WhatsApp session."
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Sync WhatsApp conversations into raw chat archives and let agents send messages.",
		Examples: []string{
			"Summarize my recent WhatsApp chats",
			"Catch me up on unread conversations",
			"Send a WhatsApp message to a contact",
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
			Description: "Scan a WhatsApp QR code to link this Jaz instance as a companion device.",
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilitySync,
			integrations.CapabilityAct,
		},
		MultiAccount: true,
		Tools:        chatTools(),
		Skills:       chatSkills(),
		ConnectionNotes: []string{
			"Jaz stores raw WhatsApp contacts and messages under the configured Jaz ingest root.",
			"Message sends are direct actions from the selected connected account.",
			"Initial history depends on what WhatsApp Web makes available during companion-device sync.",
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
			Name:        ToolReadRecent,
			Description: ToolReadRecentDescription,
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
			Description: "Guidance for safe chat sends, replies, reactions, and provider caveats.",
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
