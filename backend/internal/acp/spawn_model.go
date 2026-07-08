package acp

import (
	"context"
	"strings"
	"unicode"

	"github.com/wins/jaz/backend/internal/provider"
)

func (m *Manager) resolveAgentModelAlias(agent string, cfg AgentConfig) string {
	model := strings.TrimSpace(cfg.Model)
	if model == "" || m.cfg.ModelCatalog == nil {
		return model
	}
	key := modelAliasKey(model)
	openRouterNative := cfg.UsesProvider() && strings.EqualFold(strings.TrimSpace(cfg.ModelProvider), provider.ProviderOpenRouter)
	for _, candidate := range m.cfg.ModelCatalog.AgentModels(agent) {
		if key == modelAliasKey(candidate.Value) ||
			key == modelAliasKey(candidate.Label) ||
			key == modelAliasKey(candidate.OpenRouterID) {
			if openRouterNative {
				if id := strings.TrimSpace(candidate.OpenRouterID); id != "" {
					return id
				}
			}
			return strings.TrimSpace(candidate.Value)
		}
	}
	return model
}

func modelAliasKey(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *Manager) validateSpawnModelBeforePersist(ctx context.Context, req SpawnRequest, cfg AgentConfig) error {
	policy := agentPolicyForAgent(req.ACPAgent)
	if cfg.Local || policy.modelValidationKind == modelValidationNone {
		return nil
	}
	model := policy.sessionConfigModel(cfg)
	if strings.TrimSpace(model) == "" {
		return nil
	}
	info, err := m.probeAgentSession(ctx, req, cfg)
	if err != nil {
		return err
	}
	effective := configuredSessionModel(model)
	if policy.modelValidationKind == modelValidationClaude {
		effective = info.modelState.resolveAdvertised(effective)
	}
	return policy.validateConfiguredSessionModel(req.ACPAgent, model, effective, info.modelState)
}

func (m *Manager) probeAgentSession(ctx context.Context, req SpawnRequest, cfg AgentConfig) (acpSessionInfo, error) {
	cwd, err := m.resolveCwd(cfg.Cwd)
	if err != nil {
		return acpSessionInfo{}, err
	}
	ac, err := m.connect(ctx, req.ACPAgent, cfg, cwd, req.ArtifactSurface, req.MCPServerPolicy, req.SystemPromptExtensions)
	if err != nil {
		return acpSessionInfo{}, err
	}
	defer ac.close()
	info, err := m.newACPProtocolSession(ctx, ac, "model probe", newSessionRequest{
		Meta:       agentPolicyForAgent(req.ACPAgent).mergeSessionMeta(nil, cfg.ReasoningEffort),
		Cwd:        cwd,
		MCPServers: m.mcpServersForAgent(ctx, ac.initRaw, req.MCPServerPolicy),
	})
	if err != nil {
		return acpSessionInfo{}, err
	}
	defer m.closeProtocolSession(ac, info.response.SessionID)
	return info, nil
}
