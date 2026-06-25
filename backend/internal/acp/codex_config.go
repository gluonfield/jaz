package acp

import (
	"fmt"
	"strings"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

func codexProvider(modelProvider string, providers map[string]modelprovider.ModelProviderConfig) (modelprovider.ModelProvider, bool) {
	id := strings.ToLower(strings.TrimSpace(modelProvider))
	if id == "" || id == modelprovider.ProviderOpenAI {
		return modelprovider.ModelProvider{}, false
	}
	meta, ok := modelprovider.RunnableModelProviderByID(id)
	if !ok {
		meta = modelprovider.ModelProvider{ID: id}
	}
	if override, present := providers[id]; present {
		meta = modelprovider.ApplyModelProviderConfig(meta, override)
	}
	return meta, true
}

func codexProviderArgs(cfg AgentConfig, providers map[string]modelprovider.ModelProviderConfig) []string {
	meta, ok := codexProvider(cfg.ModelProvider, providers)
	baseURL := strings.TrimSpace(meta.BaseURL)
	envKey := strings.TrimSpace(meta.APIKeyEnv)
	if !ok || baseURL == "" || envKey == "" {
		return nil
	}
	table := "model_providers." + meta.ID
	return []string{
		"-c", fmt.Sprintf("model_provider=%q", meta.ID),
		"-c", fmt.Sprintf("%s.name=%q", table, firstNonEmpty(strings.TrimSpace(meta.Label), meta.ID)),
		"-c", fmt.Sprintf("%s.base_url=%q", table, baseURL),
		"-c", fmt.Sprintf("%s.env_key=%q", table, envKey),
		"-c", table + `.wire_api="responses"`,
	}
}
