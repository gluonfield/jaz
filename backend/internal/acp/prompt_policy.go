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
// They may still receive shared prompt modules such as memory or connections.
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

func (m *Manager) systemPrompt(ctx context.Context, cwd, artifactSurface, mcpServerPolicy string, modules promptmodule.Modules) (string, error) {
	var prompt string
	if restrictedWorkerPolicy(mcpServerPolicy) {
		base, err := m.restrictedWorkerPrompt(ctx, mcpServerPolicy)
		if err != nil {
			return "", err
		}
		prompt = base
	} else if m.cfg.SystemPrompt != nil {
		base, err := m.cfg.SystemPrompt.ACPPromptForContext(ctx, cwd, artifactSurface)
		if err != nil {
			return "", fmt.Errorf("build acp system prompt: %w", err)
		}
		prompt = base
	}
	return strings.TrimSpace(promptWithModules(prompt, modules)), nil
}

func (m *Manager) restrictedWorkerPrompt(ctx context.Context, mcpServerPolicy string) (string, error) {
	modules, ok := m.cfg.SystemPrompt.(SystemPromptModules)
	if !ok {
		return "", nil
	}
	prompt, err := modules.PromptModulesForContext(ctx, PromptModuleOptions{
		Connections: mcpServerPolicy != MCPServerPolicyBrowserWorker,
	})
	if err != nil {
		return "", fmt.Errorf("build restricted worker prompt modules: %w", err)
	}
	return prompt.Text(), nil
}
