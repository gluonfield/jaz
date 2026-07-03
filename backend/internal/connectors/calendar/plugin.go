package calendar

import "github.com/wins/jaz/backend/pkg/integrations"

const (
	ProviderID   = "google_calendar"
	ProviderName = "Google Calendar"

	ScopeEvents        = "https://www.googleapis.com/auth/calendar.events"
	ScopeUserInfoEmail = "https://www.googleapis.com/auth/userinfo.email"

	ToolGetEvents   = "google_calendar_get_events"
	ToolCreateEvent = "google_calendar_create_event"
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Read Google Calendar events and create events with guests from connected accounts.",
		Examples: []string{
			"Show my calendar for tomorrow",
			"Create a meeting with Majid next Tuesday at 2pm",
			"Find free context around my afternoon meetings",
		},
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "calendar",
		Icon: integrations.PluginIcon{
			Kind:  integrations.PluginIconKindAsset,
			Value: ProviderID,
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindOAuth,
			Description: "Jaz-managed Google OAuth for Calendar tools.",
			Scopes:      OAuthScopes,
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilityAct,
			integrations.CapabilityMCP,
		},
		MultiAccount: true,
		Tools: []integrations.PluginTool{
			tool(ToolGetEvents, "Get Google Calendar events from a connected account.", integrations.ActionRiskRead),
			tool(ToolCreateEvent, "Create a Google Calendar event and invite guests.", integrations.ActionRiskWrite),
		},
		ConnectionNotes: []string{
			"Connect each Google Calendar account separately.",
			"Guests receive invitations when create-event send_updates is all or external_only.",
			"Use primary for the main calendar, or pass another calendar ID when needed.",
		},
		Implementation: integrations.Implementation{
			Status: "available",
			Owner:  "jaz",
		},
	}
}

func tool(name, description string, risk integrations.ActionRisk) integrations.PluginTool {
	return integrations.PluginTool{
		Name:           name,
		Description:    description,
		Capability:     integrations.CapabilityAct,
		Risk:           risk,
		RequiredScopes: []string{ScopeEvents},
	}
}
