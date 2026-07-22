package acp

import (
	"encoding/json"
	"fmt"
	"strings"
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

func (m *Manager) resolveCodexCustomProviderModelMetadata(name string, cfg AgentConfig) (string, error) {
	providerID := strings.TrimSpace(cfg.ModelProvider)
	modelID := strings.TrimSpace(cfg.Model)
	usesNativeMetadata := codexNativeOpenAIProvider(providerID) ||
		strings.EqualFold(providerID, CodexProviderOpenAIAPIKey)
	if CanonicalAgentName(name) != AgentCodex || !cfg.UsesProvider() || usesNativeMetadata {
		return "", nil
	}
	if modelID == "" || m.cfg.ModelCatalog == nil {
		return "", nil
	}
	models, err := (ModelCapabilities{Catalog: m.cfg.ModelCatalog}).ProviderModels(AgentCodex, providerID)
	if err != nil {
		return "", nil
	}
	model, ok := findCapabilityModel(models, modelID)
	if !ok || model.ContextLength <= 0 {
		return "", nil
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
		return "", fmt.Errorf("encode Codex metadata for provider %q model %q: %w", providerID, modelID, err)
	}
	return string(encoded), nil
}
