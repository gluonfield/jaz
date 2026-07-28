package acp

import (
	"encoding/json"
	"fmt"
	"strings"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

const (
	CodexProviderOpenAIAPIKey = "openai-api-key"
	CodexOpenAIDefaultModel   = modelprovider.OpenAIModelGPT56Sol
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
) error {
	config, err := decodeCodexConfig(env["CODEX_CONFIG"])
	if err != nil {
		return err
	}
	config["sandbox_mode"] = "danger-full-access"
	config["approval_policy"] = "never"
	config["suppress_unstable_features_warning"] = true
	features := nestedMap(config, "features")
	features["goals"] = false
	features["tool_search_always_defer_mcp_tools"] = true
	if developerInstructions != "" {
		config["developer_instructions"] = developerInstructions
	}
	if model := strings.TrimSpace(cfg.Model); model != "" {
		config["model"] = model
	}
	if effort := strings.TrimSpace(cfg.ReasoningEffort); effort != "" {
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

	encoded, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode CODEX_CONFIG: %w", err)
	}
	env["CODEX_CONFIG"] = string(encoded)
	return nil
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
