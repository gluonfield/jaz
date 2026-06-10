package provider

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ProviderOpenRouter = "openrouter"
	ProviderOpenAI     = "openai"
	ProviderMock       = "mock"
)

type NativeProvider struct {
	ID                     string `json:"id"`
	Label                  string `json:"label"`
	BaseURL                string `json:"base_url"`
	APIKeyEnv              string `json:"api_key_env,omitempty"`
	DefaultModel           string `json:"default_model,omitempty"`
	DefaultReasoningEffort string `json:"default_reasoning_effort,omitempty"`
	Implemented            bool   `json:"implemented"`
}

func NativeProviders() []NativeProvider {
	return []NativeProvider{
		{
			ID:                     ProviderOpenRouter,
			Label:                  "OpenRouter",
			BaseURL:                "https://openrouter.ai/api/v1",
			APIKeyEnv:              "OPENROUTER_API_KEY",
			DefaultModel:           "openai/gpt-5.4-mini",
			DefaultReasoningEffort: "medium",
			Implemented:            true,
		},
		{
			ID:                     ProviderOpenAI,
			Label:                  "OpenAI",
			BaseURL:                "https://api.openai.com/v1",
			APIKeyEnv:              "OPENAI_API_KEY",
			DefaultModel:           "gpt-5.4-mini",
			DefaultReasoningEffort: "medium",
			Implemented:            true,
		},
	}
}

func NativeProviderByID(id string) (NativeProvider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, provider := range NativeProviders() {
		if provider.ID == id {
			return provider, true
		}
	}
	return NativeProvider{}, false
}

func NormalizeNativeProviderID(id string) (string, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return "", nil
	}
	if _, ok := NativeProviderByID(id); ok {
		return id, nil
	}
	return "", fmt.Errorf("unknown native provider %q; valid providers are %s", id, strings.Join(nativeProviderIDs(), ", "))
}

func nativeProviderIDs() []string {
	providers := NativeProviders()
	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID)
	}
	sort.Strings(ids)
	return ids
}
