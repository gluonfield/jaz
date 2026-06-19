package acp

import (
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/visualize"
)

func (m *Manager) spawnConfig(req SpawnRequest) (SpawnRequest, AgentConfig, string, error) {
	req.ACPAgent = CanonicalAgentName(req.ACPAgent)
	if req.ACPAgent == "" {
		req.ACPAgent = AgentJaz
	}
	req.ArtifactSurface = strings.TrimSpace(req.ArtifactSurface)
	req.MCPServerPolicy = strings.TrimSpace(req.MCPServerPolicy)
	req.SystemPromptExtensions = cleanPromptExtensions(req.SystemPromptExtensions)
	if req.MCPServerPolicy == "" && visualize.NormalizeSurface(req.ArtifactSurface) == visualize.SurfaceWidget {
		req.MCPServerPolicy = MCPServerPolicyWidget
	}
	cfg, ok, err := m.configuredAgent(req.ACPAgent)
	if err != nil {
		return SpawnRequest{}, AgentConfig{}, "", err
	}
	if !ok {
		return SpawnRequest{}, AgentConfig{}, "", fmt.Errorf("acp agent %q is not configured", req.ACPAgent)
	}
	effort := configuredAgentReasoningEffort(req.ACPAgent, cfg.ReasoningEffort)
	if req.ReasoningEffort != "" {
		effort = configuredAgentReasoningEffort(req.ACPAgent, req.ReasoningEffort)
	}
	model := strings.TrimSpace(req.Model)
	modelOverridden := model != ""
	if modelOverridden {
		cfg.Model = model
	}
	if providerID := strings.TrimSpace(req.ModelProvider); providerID != "" {
		if !modelOverridden && providerID != strings.TrimSpace(cfg.ModelProvider) && cfg.UsesProvider() {
			cfg.Model = providerDefaultModel(providerID)
		}
		cfg.ModelProvider = providerID
	}
	cfg = cfg.NormalizeProviderModel(cfg.ModelProvider)
	cfg.ReasoningEffort = effort
	return req, cfg, effort, nil
}

func providerDefaultModel(id string) string {
	meta, _ := provider.ModelProviderByID(id)
	return strings.TrimSpace(meta.DefaultModel)
}

func (m *Manager) createStoredSession(req SpawnRequest, cfg AgentConfig, effort string) (storage.Session, error) {
	modelProvider := req.ACPAgent
	if cfg.Local || cfg.UsesProvider() {
		modelProvider = strings.TrimSpace(cfg.ModelProvider)
	}
	return m.store.CreateSession(storage.CreateSession{
		Slug:            req.Slug,
		Title:           req.Title,
		ParentID:        req.ParentID,
		Runtime:         storage.RuntimeACP,
		ModelProvider:   modelProvider,
		Model:           strings.TrimSpace(cfg.Model),
		ReasoningEffort: effort,
		SourceType:      req.SourceType,
		SourceID:        req.SourceID,
		RuntimeRef: &storage.RuntimeRef{
			Type:            storage.RuntimeACP,
			Agent:           req.ACPAgent,
			ArtifactSurface: req.ArtifactSurface,
			MCPServerPolicy: req.MCPServerPolicy,
		},
	})
}

func cleanPromptExtensions(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value := strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
