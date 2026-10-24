package acp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

const (
	CodexProviderOpenAIAPIKey = "openai-api-key"
	CodexOpenAIDefaultModel   = modelprovider.OpenAIModelGPT6Astra
)

func codexProvider(modelProvider string, providers map[string]modelprovider.ModelProviderConfig) (modelprovider.ModelProvider, bool) {
	id := strings.ToLower(strings.TrimSpace(modelProvider))
	if codexNativeOpenAIProvider(id) {
		return modelprovider.ModelProvider{}, false
	}
	if id == CodexProviderOpenAIAPIKey {
		meta := modelprovider.ResolveModelProvider(modelprovider.ProviderOpenAI, providers).Meta
		if !meta.SupportsCapability(modelprovider.CapabilityResponses) {
			return modelprovider.ModelProvider{}, false
		}
		meta.ID = CodexProviderOpenAIAPIKey
		meta.Label = "OpenAI API key"
		meta.DefaultModel = CodexOpenAIDefaultModel
		return meta, true
	}
	meta := modelprovider.ResolveModelProvider(id, providers).Meta
	return meta, meta.SupportsCapability(modelprovider.CapabilityResponses)
}

func codexNativeOpenAIProvider(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	return id == "" || id == AgentCodex || id == modelprovider.ProviderOpenAI
}

func codexProviderKeyID(id string) string {
	keyID := strings.ToLower(strings.TrimSpace(id))
	if keyID == CodexProviderOpenAIAPIKey {
		return modelprovider.ProviderOpenAI
	}
	return keyID
}

func configureCodexEnv(
	env map[string]string,
	cfg AgentConfig,
	providers map[string]modelprovider.ModelProviderConfig,
	developerInstructions string,
) (func(), error) {
	config, err := decodeCodexConfig(env["CODEX_CONFIG"])
	if err != nil {
		return nil, err
	}
	config["sandbox_mode"] = "danger-full-access"
	config["approval_policy"] = "never"
	config["suppress_unstable_features_warning"] = true
	if developerInstructions != "" {
		config["developer_instructions"] = developerInstructions
	}
	// The launch names the model as `codex -m` does: Codex builds a thread's base
	// instructions for the model it starts on and tells the model about any
	// later model as a switch.
	model, effort := strings.TrimSpace(cfg.Model), strings.TrimSpace(cfg.ReasoningEffort)
	if base, suffix, ok := cutReasoningEffort(model); ok {
		model, effort = base, suffix
	}
	if model != "" {
		config["model"] = model
	}
	if effort != "" {
		config["model_reasoning_effort"] = effort
	}

	providerID, providerConfig := codexLaunchProvider(cfg, providers)
	config["model_provider"] = providerID
	if providerConfig != nil {
		provider := nestedMap(nestedMap(config, "model_providers"), providerID)
		for key, value := range providerConfig {
			provider[key] = value
		}
	}

	file, err := os.CreateTemp("", "jaz-codex-config-*.json")
	if err != nil {
		return nil, fmt.Errorf("create CODEX_CONFIG: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	writeErr := json.NewEncoder(file).Encode(config)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		cleanup()
		return nil, fmt.Errorf("write CODEX_CONFIG: %w", err)
	}
	env["CODEX_CONFIG"] = "@" + file.Name()
	return cleanup, nil
}

func codexLaunchProvider(
	cfg AgentConfig,
	providers map[string]modelprovider.ModelProviderConfig,
) (string, map[string]any) {
	if codexNativeOpenAIProvider(cfg.ModelProvider) {
		return "openai", nil
	}
	meta, ok := codexProvider(cfg.ModelProvider, providers)
	if !ok {
		return strings.ToLower(strings.TrimSpace(cfg.ModelProvider)), nil
	}
	if meta.ID == modelprovider.ProviderOllama {
		return meta.ID, nil
	}
	provider := map[string]any{
		"name":     firstNonEmpty(strings.TrimSpace(meta.Label), meta.ID),
		"base_url": strings.TrimSpace(meta.BaseURL),
		"wire_api": "responses",
	}
	if envKey := strings.TrimSpace(meta.APIKeyEnv); envKey != "" {
		provider["env_key"] = envKey
	}
	return meta.ID, provider
}

func decodeCodexConfig(raw string) (map[string]any, error) {
	if strings.HasPrefix(raw, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return nil, fmt.Errorf("read CODEX_CONFIG: %w", err)
		}
		raw = string(data)
	}
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, fmt.Errorf("parse CODEX_CONFIG: %w", err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}
