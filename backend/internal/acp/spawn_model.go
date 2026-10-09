package acp

import (
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
	capabilities := ModelCapabilities{Catalog: m.cfg.ModelCatalog}
	candidates := capabilities.AgentModels(agent)
	if cfg.UsesProvider() {
		var err error
		candidates, err = capabilities.AgentModelsForProvider(agent, cfg.ModelProvider)
		if err != nil {
			return model
		}
	}
	for _, candidate := range candidates {
		if key == modelAliasKey(candidate.Value) ||
			key == modelAliasKey(candidate.Label) ||
			key == modelAliasKey(candidate.OpenRouterID) {
			if openRouterNative && candidate.OpenRouterID != "" {
				return candidate.OpenRouterID
			}
			return candidate.Value
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
