package acp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

const openCodeOpenAICompatibleNPM = "@ai-sdk/openai-compatible"

type openCodeConfigContent struct {
	Instructions []string                          `json:"instructions,omitempty"`
	Permission   string                            `json:"permission"`
	Provider     map[string]openCodeProviderConfig `json:"provider,omitempty"`
	Model        string                            `json:"model,omitempty"`
	SmallModel   string                            `json:"small_model,omitempty"`
}

type openCodeProviderConfig struct {
	API     string                         `json:"api,omitempty"`
	Name    string                         `json:"name,omitempty"`
	Env     []string                       `json:"env,omitempty"`
	NPM     string                         `json:"npm,omitempty"`
	Options map[string]string              `json:"options,omitempty"`
	Models  map[string]openCodeModelConfig `json:"models,omitempty"`
}

type openCodeModelConfig struct {
	ID       string                                `json:"id,omitempty"`
	Variants map[string]openCodeModelVariantConfig `json:"variants,omitempty"`
}

type openCodeModelVariantConfig struct {
	Reasoning *openCodeReasoningConfig `json:"reasoning,omitempty"`
}

type openCodeReasoningConfig struct {
	Effort string `json:"effort,omitempty"`
}

func (m *Manager) configureOpenCode(env map[string]string, agent AgentConfig, prompt string) (cleanup func(), err error) {
	cleanup = func() {}
	if strings.TrimSpace(env["OPENCODE_CONFIG_CONTENT"]) != "" {
		return cleanup, nil
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	content := openCodeConfigContent{Permission: "allow"}
	if prompt != "" {
		file, err := os.CreateTemp(env["OPENCODE_CONFIG_DIR"], "jaz-instructions-*.md")
		if err != nil {
			return cleanup, fmt.Errorf("create opencode instructions: %w", err)
		}
		cleanup = func() {
			_ = os.Remove(file.Name())
		}
		_, writeErr := file.WriteString(prompt + "\n")
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return cleanup, fmt.Errorf("write opencode instructions: %w", err)
		}
		content.Instructions = []string{file.Name()}
	}
	root := firstNonEmpty(m.cfg.Root, filepath.Join(os.TempDir(), "jaz"))
	auth := resolveAgentAuthWithProviders(AgentOpenCode, agent, root, env, m.providers())
	keyEnv, _, _ := auth.APIKeyBinding()
	model := agent.ProviderQualifiedModel()
	if providerConfig, ok := m.openCodeProviderConfig(model); ok {
		providerID := modelprovider.OpenCodeProviderIDFromModel(model)
		content.Provider = map[string]openCodeProviderConfig{providerID: providerConfig}
	}
	if model != "" {
		content.Model = model
		if modelprovider.OpenCodeProviderIDFromModel(model) == modelprovider.ProviderOpenRouter {
			content.SmallModel = model
		}
	}
	if keyEnv != "" {
		// OpenCode lets a stored `opencode auth` key override the environment,
		// but options.apiKey outranks both, so the key Jaz binds is the one used.
		providerID := openCodeProviderID(model)
		provider := content.Provider[providerID]
		provider.Options = map[string]string{"apiKey": "{env:" + keyEnv + "}"}
		if content.Provider == nil {
			content.Provider = map[string]openCodeProviderConfig{}
		}
		content.Provider[providerID] = provider
	}
	addOpenCodeReasoningVariant(&content, model, agent.ReasoningEffort)
	data, err := json.Marshal(content)
	if err != nil {
		return cleanup, err
	}
	env["OPENCODE_CONFIG_CONTENT"] = string(data)
	return cleanup, nil
}

func (m *Manager) openCodeProviderConfig(model string) (openCodeProviderConfig, bool) {
	providerID, modelID := modelprovider.SplitProviderModel(model)
	if providerID == "" {
		return openCodeProviderConfig{}, false
	}
	provider := modelprovider.ResolveModelProvider(providerID, m.providers())
	if !provider.Meta.SupportsCapability(modelprovider.CapabilityChatCompletions) {
		return openCodeProviderConfig{}, false
	}
	if !shouldWriteOpenCodeProviderConfig(provider.Config, provider.Meta, provider.Configured, provider.BuiltIn) {
		return openCodeProviderConfig{}, false
	}
	baseURL := firstNonEmpty(provider.Config.BaseURL, provider.Meta.BaseURL)
	if baseURL == "" {
		return openCodeProviderConfig{}, false
	}
	key := modelprovider.ConfiguredAPIKeyEnv(providerID, provider.Config)
	result := openCodeProviderConfig{
		API:  baseURL,
		Name: firstNonEmpty(provider.Config.Label, provider.Meta.Label, providerID),
		NPM:  openCodeOpenAICompatibleNPM,
	}
	if key != "" {
		result.Env = []string{key}
	}
	if modelID != "" {
		result.Models = map[string]openCodeModelConfig{modelID: {}}
	}
	return result, true
}

func addOpenCodeReasoningVariant(content *openCodeConfigContent, model, effort string) {
	providerID, modelID := modelprovider.SplitProviderModel(model)
	effort, err := NormalizeAgentReasoningEffort(AgentOpenCode, effort)
	if err != nil || providerID != modelprovider.ProviderOpenRouter || modelID == "" || effort == "" {
		return
	}
	if content.Provider == nil {
		content.Provider = map[string]openCodeProviderConfig{}
	}
	provider := content.Provider[providerID]
	if provider.Models == nil {
		provider.Models = map[string]openCodeModelConfig{}
	}
	modelConfig := provider.Models[modelID]
	if modelConfig.Variants == nil {
		modelConfig.Variants = map[string]openCodeModelVariantConfig{}
	}
	modelConfig.Variants[effort] = openCodeModelVariantConfig{
		Reasoning: &openCodeReasoningConfig{Effort: effort},
	}
	provider.Models[modelID] = modelConfig
	content.Provider[providerID] = provider
}

func shouldWriteOpenCodeProviderConfig(cfg modelprovider.ModelProviderConfig, meta modelprovider.ModelProvider, configured, builtIn bool) bool {
	if !builtIn {
		return configured && strings.TrimSpace(cfg.BaseURL) != ""
	}
	if meta.OpenAICompatible {
		return true
	}
	if !configured {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Type), "openai-compatible") {
		return strings.TrimSpace(firstNonEmpty(cfg.BaseURL, meta.BaseURL)) != ""
	}
	if value := strings.TrimSpace(cfg.BaseURL); value != "" && value != strings.TrimSpace(meta.BaseURL) {
		return true
	}
	if value := strings.TrimSpace(cfg.APIKeyEnv); value != "" && value != strings.TrimSpace(meta.APIKeyEnv) {
		return true
	}
	return false
}
