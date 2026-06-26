package acp

import (
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/promptmodule"
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
	req.SystemPromptExtensions = promptmodule.New(req.SystemPromptExtensions...)
	if req.MCPServerPolicy == "" {
		req.MCPServerPolicy = mcpServerPolicyForSourceType(req.SourceType)
	}
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
			cfg.Model = agentProviderDefaultModel(req.ACPAgent, providerID, m.providers())
		}
		cfg.ModelProvider = providerID
	}
	cfg = cfg.NormalizeProviderModel(cfg.ModelProvider)
	if strings.EqualFold(strings.TrimSpace(req.ReasoningEffort), "none") && agentPolicyForAgent(req.ACPAgent).effortEncodedInModel(cfg.Model) {
		cfg.Model = strings.TrimSpace(cfg.Model[:strings.LastIndex(cfg.Model, "/")])
	}
	cfg.ReasoningEffort = effort
	return req, cfg, effort, nil
}

func agentProviderDefaultModel(agent, id string, providers map[string]provider.ModelProviderConfig) string {
	if CanonicalAgentName(agent) == AgentCodex {
		if meta, ok := codexProvider(id, providers); ok {
			return strings.TrimSpace(meta.DefaultModel)
		}
	}
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
