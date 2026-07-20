package acp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
)

const codexModelMetadataEnv = "JAZ_CODEX_MODEL_METADATA"

type codexModelMetadata struct {
	ID                     string   `json:"id"`
	DisplayName            string   `json:"display_name"`
	Description            string   `json:"description,omitempty"`
	ContextWindow          int      `json:"context_window"`
	InputModalities        []string `json:"input_modalities,omitempty"`
	ReasoningEfforts       []string `json:"reasoning_efforts,omitempty"`
	DefaultReasoningEffort string   `json:"default_reasoning_effort,omitempty"`
}

func (m *Manager) resolveCodexModelMetadata(name string, cfg AgentConfig) (string, error) {
	providerID := strings.TrimSpace(cfg.ModelProvider)
	modelID := strings.TrimSpace(cfg.Model)
	if CanonicalAgentName(name) != AgentCodex || !cfg.UsesProvider() || !strings.EqualFold(providerID, provider.ProviderOpenRouter) {
		return "", nil
	}
	if modelID == "" {
		return "", fmt.Errorf("resolve Codex OpenRouter metadata: model is empty")
	}
	if m.cfg.ModelCatalog == nil {
		return "", fmt.Errorf("resolve Codex OpenRouter metadata for %q: model catalog is unavailable", modelID)
	}
	models, err := (ModelCapabilities{Catalog: m.cfg.ModelCatalog}).ProviderModels(AgentCodex, provider.ProviderOpenRouter)
	if err != nil {
		return "", fmt.Errorf("resolve Codex OpenRouter metadata for %q: %w", modelID, err)
	}
	model, ok := findCapabilityModel(models, modelID)
	if !ok {
		return "", fmt.Errorf("resolve Codex OpenRouter metadata: model %q is not in the provider catalog", modelID)
	}
	if model.ContextLength <= 0 {
		return "", fmt.Errorf("resolve Codex OpenRouter metadata: model %q has no context window", modelID)
	}
	metadata := codexModelMetadata{
		ID:                     modelID,
		DisplayName:            model.Label,
		Description:            model.Description,
		ContextWindow:          model.ContextLength,
		InputModalities:        append([]string(nil), model.InputModalities...),
		ReasoningEfforts:       append([]string(nil), model.Reasoning.Efforts...),
		DefaultReasoningEffort: model.Reasoning.DefaultEffort,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode Codex OpenRouter metadata for %q: %w", modelID, err)
	}
	return string(encoded), nil
}
