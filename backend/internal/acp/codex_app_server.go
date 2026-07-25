package acp

import (
	"encoding/json"
	"fmt"
	"strings"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

const codexAppServerAdapter = "codex-app-server"

func isCodexAppServer(name string, cfg AgentConfig) bool {
	return CanonicalAgentName(name) == AgentCodex &&
		cfg.ManagedAdapter == codexAppServerAdapter
}

func systemPromptAtLaunch(name string, cfg AgentConfig) bool {
	return agentPolicyForAgent(name).systemPromptAtLaunch || isCodexAppServer(name, cfg)
}

func configureCodexAppServerEnv(
	env map[string]string,
	cfg AgentConfig,
	providers map[string]modelprovider.ModelProviderConfig,
	developerInstructions string,
) error {
	config, err := decodeCodexAppServerConfig(env["CODEX_CONFIG"])
	if err != nil {
		return err
	}
	config["sandbox_mode"] = "danger-full-access"
	config["approval_policy"] = "never"
	config["suppress_unstable_features_warning"] = true
	config["features"] = mergeAnyMap(config["features"], map[string]any{
		"goals":                              false,
		"tool_search_always_defer_mcp_tools": true,
	})
	if developerInstructions != "" {
		config["developer_instructions"] = developerInstructions
	}
	if model := strings.TrimSpace(cfg.Model); model != "" {
		config["model"] = model
	}
	if effort := strings.TrimSpace(cfg.ReasoningEffort); effort != "" {
		config["model_reasoning_effort"] = effort
	}

	providerID, providerConfig := codexAppServerProvider(cfg, providers)
	config["model_provider"] = providerID
	if providerConfig != nil {
		modelProviders := mergeAnyMap(config["model_providers"], nil)
		modelProviders[providerID] = mergeAnyMap(modelProviders[providerID], providerConfig)
		config["model_providers"] = modelProviders
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode CODEX_CONFIG: %w", err)
	}
	env["CODEX_CONFIG"] = string(encoded)
	return nil
}

func codexAppServerProvider(
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

func decodeCodexAppServerConfig(raw string) (map[string]any, error) {
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

func mergeAnyMap(current any, values map[string]any) map[string]any {
	out := map[string]any{}
	if existing, ok := current.(map[string]any); ok {
		for key, value := range existing {
			out[key] = value
		}
	}
	for key, value := range values {
		out[key] = value
	}
	return out
}
