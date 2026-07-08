package jaz

import "github.com/wins/jaz/backend/pkg/integrations"

const (
	ProviderID   = "jaz"
	ProviderName = "Jaz"
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Jaz's built-in tools for memory, spawned agents, threads, loops, goals, browsing, and visualisation.",
		Examples: []string{
			"Search my memory for the launch plan",
			"Spawn a coding agent on this repo",
			"Create a loop that checks CI every morning",
		},
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "assistant",
		Icon: integrations.PluginIcon{
			Kind:  integrations.PluginIconKindAsset,
			Value: "jaz",
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindInternal,
			Description: "Built into Jaz and exposed to every agent automatically. No sign-in.",
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilityAct,
			integrations.CapabilityMCP,
		},
		// Thread-surface tool metadata; pinned to the live jaztools
		// registrations by that package's plugin sync test.
		Tools: []integrations.PluginTool{
			{Name: "memory_search", Description: "Search Jaz's persistent memory and return cited results.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "memory_get_page", Description: "Read a memory page's raw markdown.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "thread_context", Description: "Read context from another Jaz thread.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "agent_spawn", Description: "Spawn an ACP coding agent session.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "agent_send", Description: "Send a follow-up prompt to a spawned agent.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "agent_wait", Description: "Wait for a spawned agent to finish its turn.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "agent_status", Description: "Check a spawned agent's status.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "agent_cancel", Description: "Cancel a spawned agent.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "agent_list", Description: "List spawned agent sessions.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "loop_list", Description: "List recurring loops.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "loop_get", Description: "Inspect a loop and its recent runs.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "loop_boards", Description: "List loop boards.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "loop_create", Description: "Create a recurring loop.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "loop_update", Description: "Update a loop.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "loop_run", Description: "Trigger a loop run now.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "loop_delete", Description: "Delete a loop.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskDelete},
			{Name: "create_goal", Description: "Create a goal for the current session.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "get_goal", Description: "Read the current session goal.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "update_goal", Description: "Update the current session goal.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "browser_do", Description: "Delegate a browser action task to a browser worker.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
			{Name: "browser_get", Description: "Retrieve information through a browser worker.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "browser_check", Description: "Verify something through a browser worker.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "visualise_read_me", Description: "Read the inline artifact authoring guide.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskRead},
			{Name: "visualise_show_widget", Description: "Render an inline widget artifact in the thread.", Capability: integrations.CapabilityAct, Risk: integrations.ActionRiskWrite},
		},
		ConnectionNotes: []string{
			"Built into Jaz. Every agent gets these tools automatically; there is nothing to connect.",
		},
		Implementation: integrations.Implementation{
			Status: "available",
			Owner:  "jaz",
		},
	}
}
