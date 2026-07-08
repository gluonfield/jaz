package provider

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
)

const (
	ProviderOpenRouter = "openrouter"
	ProviderOpenAI     = "openai"
	ProviderOllama     = "ollama"
	ProviderMock       = "mock"

	DefaultOpenRouterModel = "z-ai/glm-5.2"
	DefaultOpenAIModel     = "gpt-5.4-mini"

	CapabilityJaz      = "jaz"
	CapabilityOpenCode = "opencode"
	CapabilityCodex    = "codex"
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
	Codex                  bool   `json:"codex,omitempty"`
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
	Codex     bool
}

func ModelProviders() []ModelProvider {
	return []ModelProvider{
		{
			ID:                     ProviderOpenRouter,
			Label:                  "OpenRouter",
			BaseURL:                "https://openrouter.ai/api/v1",
			APIKeyEnv:              "OPENROUTER_API_KEY",
			DefaultModel:           DefaultOpenRouterModel,
			DefaultReasoningEffort: "medium",
			Implemented:            true,
			OpenCode:               true,
			Codex:                  true,
			RequiresAPIKey:         true,
		},
		{
			ID:                     ProviderOpenAI,
			Label:                  "OpenAI",
			BaseURL:                "https://api.openai.com/v1",
			APIKeyEnv:              "OPENAI_API_KEY",
			DefaultModel:           DefaultOpenAIModel,
			DefaultReasoningEffort: "medium",
			Implemented:            true,
			OpenCode:               true,
			Codex:                  true,
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

func RunnableModelProviders() []ModelProvider {
	out := []ModelProvider{}
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

func RunnableModelProviderByID(id string) (ModelProvider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, provider := range RunnableModelProviders() {
		if provider.ID == id {
			return provider, true
		}
	}
	return ModelProvider{}, false
}

func OpenCodeProviderByID(id string) (ModelProvider, bool) {
	provider, ok := ModelProviderByID(id)
	return provider, ok && provider.OpenCode
}

func ApplyModelProviderConfig(meta ModelProvider, cfg ModelProviderConfig) ModelProvider {
	_, builtIn := ModelProviderByID(meta.ID)
	if strings.TrimSpace(cfg.Label) != "" {
		meta.Label = cfg.Label
	}
	if strings.TrimSpace(meta.Label) == "" {
		meta.Label = meta.ID
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		meta.BaseURL = cfg.BaseURL
	}
	if strings.TrimSpace(cfg.APIKeyEnv) != "" {
		meta.APIKeyEnv = cfg.APIKeyEnv
	}
	if cfg.OpenCode {
		meta.OpenCode = true
	}
	if cfg.Codex {
		meta.Codex = true
	}
	if !builtIn && strings.TrimSpace(cfg.BaseURL) != "" {
		meta.OpenCode = true
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Type), "openai-compatible") {
		meta.OpenCode = true
		meta.Codex = true
		meta.OpenAICompatible = true
	}
	if strings.TrimSpace(meta.APIKeyEnv) == "" && needsGeneratedAPIKeyEnv(builtIn, meta, cfg) {
		meta.APIKeyEnv = ConfiguredAPIKeyEnv(meta.ID, cfg)
	}
	if strings.TrimSpace(cfg.APIKey) != "" || strings.TrimSpace(meta.APIKeyEnv) != "" {
		meta.RequiresAPIKey = true
	}
	return meta
}

func ModelProviderConfigPresent(cfg ModelProviderConfig) bool {
	return strings.TrimSpace(cfg.Type) != "" ||
		strings.TrimSpace(cfg.Label) != "" ||
		strings.TrimSpace(cfg.BaseURL) != "" ||
		strings.TrimSpace(cfg.APIKey) != "" ||
		cfg.OpenCode ||
		cfg.Codex
}

func needsGeneratedAPIKeyEnv(builtIn bool, meta ModelProvider, cfg ModelProviderConfig) bool {
	if strings.TrimSpace(cfg.APIKey) != "" || strings.TrimSpace(cfg.APIKeyEnv) != "" {
		return true
	}
	if builtIn {
		return false
	}
	return !BaseURLIsLoopback(meta.BaseURL)
}

func BaseURLIsLoopback(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (p ModelProvider) SupportsCapability(capability string) bool {
	switch strings.TrimSpace(capability) {
	case CapabilityJaz:
		return p.Implemented
	case CapabilityOpenCode:
		return p.OpenCode
	case CapabilityCodex:
		return p.Codex
	default:
		return false
	}
}

func ConfiguredAPIKeyEnv(id string, cfg ModelProviderConfig) string {
	if key := strings.TrimSpace(cfg.APIKeyEnv); key != "" {
		return key
	}
	if meta, ok := ModelProviderByID(id); ok && strings.TrimSpace(meta.APIKeyEnv) != "" {
		return meta.APIKeyEnv
	}
	if ModelProviderConfigPresent(cfg) {
		return "JAZ_PROVIDER_" + EnvKeyName(id) + "_API_KEY"
	}
	return ""
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

func EnvKeyName(id string) string {
	id = strings.ToUpper(strings.TrimSpace(id))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range id {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func NormalizeRunnableModelProviderID(id string) (string, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return "", nil
	}
	if _, ok := RunnableModelProviderByID(id); ok {
		return id, nil
	}
	return "", fmt.Errorf("unknown model provider %q; valid providers are %s", id, strings.Join(runnableModelProviderIDs(), ", "))
}

func runnableModelProviderIDs() []string {
	providers := RunnableModelProviders()
	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID)
	}
	sort.Strings(ids)
	return ids
}
