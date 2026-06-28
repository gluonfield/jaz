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
// policy-named jaztools surface, and without the base agent system prompt.
// Adding a worker is a single entry here.
var restrictedWorkerPolicies = map[string]string{
	storage.SourceMemorySearch: MCPServerPolicyMemorySearchWorker,
	storage.SourceMemorySource: MCPServerPolicyMemorySourceWorker,
	storage.SourceBrowserTask:  MCPServerPolicyBrowserWorker,
}

func mcpServerPolicyForSourceType(sourceType string) string {
	return restrictedWorkerPolicies[sourceType]
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
		return session.RuntimeRef.MCPServerPolicy
	}
	return mcpServerPolicyForSourceType(session.SourceType)
}

func configForMCPServerPolicy(agent string, cfg AgentConfig, policy string) AgentConfig {
	if CanonicalAgentName(agent) != AgentCodex || !restrictedWorkerPolicy(policy) {
		return cfg
	}
	cfg.Args = withoutCodexConfig(cfg.Args, "features.tool_search_always_defer_mcp_tools")
	cfg.ManagedAdapterArgs = withoutCodexConfig(cfg.ManagedAdapterArgs, "features.tool_search_always_defer_mcp_tools")
	for _, key := range []string{"features.browser_use", "features.browser_use_external", "features.in_app_browser"} {
		cfg.Args = withCodexConfig(cfg.Args, key, "false")
		cfg.ManagedAdapterArgs = withCodexConfig(cfg.ManagedAdapterArgs, key, "false")
	}
	return cfg
}

func withoutCodexConfig(args []string, key string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "-c" && i+1 < len(args) && codexConfigArgKey(args[i+1]) == key {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-c=") && codexConfigArgKey(strings.TrimPrefix(arg, "-c=")) == key {
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func codexConfigArgKey(arg string) string {
	arg = strings.TrimSpace(arg)
	key, _, _ := strings.Cut(arg, "=")
	return strings.TrimSpace(key)
}

func withCodexConfig(args []string, key, value string) []string {
	args = withoutCodexConfig(args, key)
	return append(args, "-c", key+"="+value)
}

func (m *Manager) systemPrompt(ctx context.Context, cwd, artifactSurface, mcpServerPolicy string, modules promptmodule.Modules) (string, error) {
	var prompt string
	if m.cfg.SystemPrompt != nil && !restrictedWorkerPolicy(mcpServerPolicy) {
		base, err := m.cfg.SystemPrompt.ACPPromptForContext(ctx, cwd, artifactSurface)
		if err != nil {
			return "", fmt.Errorf("build acp system prompt: %w", err)
		}
		prompt = base
	}
	return strings.TrimSpace(promptWithModules(prompt, modules)), nil
}
