package acp

import (
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
	if id == "" || id == AgentCodex || id == modelprovider.ProviderOpenAI {
		return modelprovider.ModelProvider{}, false
	}
	if id == CodexProviderOpenAIAPIKey {
		meta := resolveModelProvider(modelprovider.ProviderOpenAI, providers).meta
		meta.ID = CodexProviderOpenAIAPIKey
		meta.Label = "OpenAI API key"
		meta.DefaultModel = CodexOpenAIDefaultModel
		return meta, true
	}
	meta := resolveModelProvider(id, providers).meta
	return meta, meta.SupportsCapability(modelprovider.CapabilityResponses)
}

func codexProviderKeyID(id string) string {
	keyID := strings.ToLower(strings.TrimSpace(id))
	if keyID == CodexProviderOpenAIAPIKey {
		return modelprovider.ProviderOpenAI
	}
	return keyID
}

func codexProviderArgs(cfg AgentConfig, providers map[string]modelprovider.ModelProviderConfig) []string {
	meta, ok := codexProvider(cfg.ModelProvider, providers)
	if ok && meta.ID == modelprovider.ProviderOllama {
		return []string{"-c", `model_provider="ollama"`}
	}
	baseURL := strings.TrimSpace(meta.BaseURL)
	envKey := strings.TrimSpace(meta.APIKeyEnv)
	if !ok || baseURL == "" {
		return nil
	}
	table := "model_providers." + meta.ID
	args := []string{
		"-c", fmt.Sprintf("model_provider=%q", meta.ID),
		"-c", fmt.Sprintf("%s.name=%q", table, firstNonEmpty(strings.TrimSpace(meta.Label), meta.ID)),
		"-c", fmt.Sprintf("%s.base_url=%q", table, baseURL),
	}
	if envKey != "" {
		args = append(args, "-c", fmt.Sprintf("%s.env_key=%q", table, envKey))
	}
	return append(args, "-c", table+`.wire_api="responses"`)
}
