package provider

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ProviderOpenRouter = "openrouter"
	ProviderOpenAI     = "openai"
	ProviderOllama     = "ollama"
	ProviderMock       = "mock"
)

type ModelProvider struct {
	ID                     string `json:"id"`
	Label                  string `json:"label"`
	BaseURL                string `json:"base_url"`
	APIKeyEnv              string `json:"api_key_env,omitempty"`
	DefaultModel           string `json:"default_model,omitempty"`
	DefaultReasoningEffort string `json:"default_reasoning_effort,omitempty"`
	Implemented            bool   `json:"implemented"`
	OpenCode               bool   `json:"opencode,omitempty"`
	OpenAICompatible       bool   `json:"openai_compatible,omitempty"`
	RequiresAPIKey         bool   `json:"requires_api_key,omitempty"`
}

type ModelProviderConfig struct {
	Type      string
	Label     string
	BaseURL   string
	APIKey    string
	APIKeyEnv string
	OpenCode  bool
}

type NativeProvider = ModelProvider

func ModelProviders() []ModelProvider {
	return []ModelProvider{
		{
			ID:                     ProviderOpenRouter,
			Label:                  "OpenRouter",
			BaseURL:                "https://openrouter.ai/api/v1",
			APIKeyEnv:              "OPENROUTER_API_KEY",
			DefaultModel:           "openai/gpt-5.4-mini",
			DefaultReasoningEffort: "medium",
			Implemented:            true,
			OpenCode:               true,
			RequiresAPIKey:         true,
		},
		{
			ID:                     ProviderOpenAI,
			Label:                  "OpenAI",
			BaseURL:                "https://api.openai.com/v1",
			APIKeyEnv:              "OPENAI_API_KEY",
			DefaultModel:           "gpt-5.4-mini",
			DefaultReasoningEffort: "medium",
			Implemented:            true,
			OpenCode:               true,
			RequiresAPIKey:         true,
		},
		{
			ID:               ProviderOllama,
			Label:            "Ollama",
			BaseURL:          "http://localhost:11434/v1",
			OpenCode:         true,
			OpenAICompatible: true,
		},
	}
}

func NativeProviders() []NativeProvider {
	out := []NativeProvider{}
	for _, provider := range ModelProviders() {
		if provider.Implemented {
			out = append(out, provider)
		}
	}
	return out
}

func ModelProviderByID(id string) (ModelProvider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, provider := range ModelProviders() {
		if provider.ID == id {
			return provider, true
		}
	}
	return ModelProvider{}, false
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

func OpenCodeProviderByID(id string) (ModelProvider, bool) {
	provider, ok := ModelProviderByID(id)
	return provider, ok && provider.OpenCode
}

func OpenCodeProviderIDFromModel(model string) string {
	providerID, _ := SplitProviderModel(model)
	return providerID
}

func SplitProviderModel(model string) (string, string) {
	model = strings.TrimSpace(model)
	before, after, ok := strings.Cut(model, "/")
	if !ok {
		return "", model
	}
	return strings.ToLower(strings.TrimSpace(before)), strings.TrimSpace(after)
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
