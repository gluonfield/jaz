package acp

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/storage"
)

// restrictedWorkerPolicies maps a worker session's source type to its MCP
// server policy. These sessions run with only the jaztools server, on the
// policy-named jaztools surface, and without the full base agent system prompt.
// Adding a worker is a single entry here.
var restrictedWorkerPolicies = map[string]string{
	storage.SourceMemorySearch:      MCPServerPolicyMemorySearchWorker,
	storage.SourceMemorySource:      MCPServerPolicyMemorySourceWorker,
	storage.SourceMemoryDream:       MCPServerPolicyMemorySourceWorker,
	storage.LegacySourceBrowserTask: MCPServerPolicyRetiredWorker,
}

// mcpServerPolicyForSourceType is a worker's restricted policy, the bot
// policy for a bot's own thread, or every server.
func mcpServerPolicyForSourceType(sourceType string) string {
	if sourceType == storage.SourceBot || sourceType == storage.SourceBotMember {
		return MCPServerPolicyBot
	}
	return restrictedWorkerPolicies[sourceType]
}

// watchedSourceType reports whether a person writes a session's next prompt:
// in a user's thread or a bot's chat. Nobody writes to a bot's subtasks or
// group seats, routine runs or memory workers.
func watchedSourceType(sourceType string) bool {
	return sourceType == "" || sourceType == storage.SourceBot
}

// rendersPromptOnLaunch reports whether the agent renders the prompt it is
// given on every launch rather than the one recorded at the conversation's
// start: a Claude bot's, since a bot keeps one conversation for good. Its
// relaunch therefore rewrites its prompt cache; other threads keep native
// caching.
func rendersPromptOnLaunch(agent, mcpServerPolicy string) bool {
	return CanonicalAgentName(agent) == AgentClaude && mcpServerPolicy == MCPServerPolicyBot
}

// fullMCPPolicy reports whether a policy gives a session every MCP server.
func fullMCPPolicy(policy string) bool {
	return policy == MCPServerPolicyAll || policy == MCPServerPolicyWidget || policy == MCPServerPolicyBot
}

func restrictedWorkerPolicy(policy string) bool {
	for _, workerPolicy := range restrictedWorkerPolicies {
		if workerPolicy == policy {
			return true
		}
	}
	return false
}

func effectiveMCPServerPolicy(session storage.Session) string {
	if session.RuntimeRef != nil && session.RuntimeRef.MCPServerPolicy != "" {
		if session.RuntimeRef.MCPServerPolicy == "browser_worker" {
			return MCPServerPolicyRetiredWorker
		}
		return session.RuntimeRef.MCPServerPolicy
	}
	return mcpServerPolicyForSourceType(session.SourceType)
}

func (m *Manager) systemPrompt(ctx context.Context, cwd, artifactSurface, mcpServerPolicy string, modules promptmodule.Modules) (string, error) {
	var prompt string
	if !restrictedWorkerPolicy(mcpServerPolicy) && m.cfg.SystemPrompt != nil {
		base, err := m.cfg.SystemPrompt.ACPPromptForContext(ctx, cwd, artifactSurface)
		if err != nil {
			return "", fmt.Errorf("build acp system prompt: %w", err)
		}
		prompt = base
	}
	return strings.TrimSpace(promptWithModules(prompt, modules)), nil
}
