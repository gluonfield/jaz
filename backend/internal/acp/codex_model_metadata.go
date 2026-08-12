package acp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/modelcatalog"
)

const codexModelMetadataEnv = "JAZ_CODEX_MODEL_METADATA"

type codexModelMetadata struct {
	ID                     string   `json:"id"`
	DisplayName            string   `json:"display_name"`
	Description            string   `json:"description,omitempty"`
	ContextWindow          int      `json:"context_window"`
	InputModalities        []string `json:"input_modalities,omitempty"`
	ReasoningEfforts       []string `json:"reasoning_efforts"`
	DefaultReasoningEffort string   `json:"default_reasoning_effort,omitempty"`
}

func (m *Manager) resolveCodexCustomProviderModelMetadata(name string, cfg AgentConfig) (string, error) {
	providerID := strings.TrimSpace(cfg.ModelProvider)
	modelID := strings.TrimSpace(cfg.Model)
	if CanonicalAgentName(name) != AgentCodex || !cfg.UsesProvider() || agentOwnsModelMetadata(name, providerID) {
		return "", nil
	}
	if modelID == "" {
		return "", fmt.Errorf("Codex provider %q requires an explicit model", providerID)
	}
	if m.cfg.ModelCatalog == nil {
		return "", fmt.Errorf("Codex provider %q model %q requires model metadata", providerID, modelID)
	}
	models, err := (ModelCapabilities{Catalog: m.cfg.ModelCatalog}).ProviderModels(AgentCodex, providerID)
	if err != nil {
		return "", fmt.Errorf("resolve Codex metadata for provider %q model %q: %w", providerID, modelID, err)
	}
	model, ok := findCapabilityModel(models, modelID)
	if !ok {
		return "", fmt.Errorf("Codex provider %q has no metadata for model %q", providerID, modelID)
	}
	if model.ContextLength <= 0 || len(model.InputModalities) == 0 || model.Reasoning.Status != modelcatalog.ReasoningReady {
		return "", fmt.Errorf("Codex provider %q has incomplete metadata for model %q", providerID, modelID)
	}
	metadata := codexModelMetadata{
		ID:                     modelID,
		DisplayName:            model.Label,
		Description:            model.Description,
		ContextWindow:          model.ContextLength,
		InputModalities:        append([]string(nil), model.InputModalities...),
		ReasoningEfforts:       append([]string{}, model.Reasoning.Efforts...),
		DefaultReasoningEffort: model.Reasoning.DefaultEffort,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode Codex metadata for provider %q model %q: %w", providerID, modelID, err)
	}
	return string(encoded), nil
}
