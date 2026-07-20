package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/promptmodule"
	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
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
	API    string                         `json:"api,omitempty"`
	Name   string                         `json:"name,omitempty"`
	Env    []string                       `json:"env,omitempty"`
	NPM    string                         `json:"npm,omitempty"`
	Models map[string]openCodeModelConfig `json:"models,omitempty"`
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

func (m *Manager) loadOpenCodeProviderEnv(env map[string]string, root string) {
	for _, key := range modelProviderEnvNames(m.providers()) {
		loadRuntimeEnvKey(env, root, key)
	}
	for id, cfg := range m.providers() {
		if strings.TrimSpace(cfg.APIKey) == "" {
			continue
		}
		key := openCodeConfiguredProviderEnv(id, cfg)
		if key != "" && strings.TrimSpace(env[key]) == "" {
			env[key] = cfg.APIKey
		}
	}
}

func modelProviderEnvNames(configs map[string]modelprovider.ModelProviderConfig) []string {
	keys := map[string]struct{}{}
	for _, provider := range modelprovider.ModelProviders() {
		if provider.SupportsCapability(modelprovider.CapabilityChatCompletions) && strings.TrimSpace(provider.APIKeyEnv) != "" {
			keys[provider.APIKeyEnv] = struct{}{}
			if alias := apiKeyAlias(provider.APIKeyEnv); alias != "" {
				keys[alias] = struct{}{}
			}
		}
	}
	for id, cfg := range configs {
		if key := openCodeConfiguredProviderEnv(id, cfg); key != "" {
			keys[key] = struct{}{}
			if alias := apiKeyAlias(key); alias != "" {
				keys[alias] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func loadRuntimeEnvKey(env map[string]string, root, key string) {
	key = strings.TrimSpace(key)
	if key == "" || strings.TrimSpace(env[key]) != "" {
		return
	}
	if value, ok := runtimeenv.Lookup(runtimeenv.Path(root), key); ok {
		env[key] = value
	}
}

func (m *Manager) prepareOpenCodeConfig(ctx context.Context, env map[string]string, agent AgentConfig, cwd, artifactSurface, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) error {
	if strings.TrimSpace(env["OPENCODE_CONFIG_CONTENT"]) != "" {
		return nil
	}
	content := openCodeConfigContent{Permission: "allow"}
	if instruction, err := m.prepareOpenCodeInstructionFile(ctx, env, cwd, artifactSurface, mcpServerPolicy, systemPromptExtensions); err != nil {
		return err
	} else if instruction != "" {
		content.Instructions = []string{instruction}
	}
	model := agent.ProviderQualifiedModel()
	if providerConfig, ok := m.openCodeProviderConfig(env, model); ok {
		providerID := modelprovider.OpenCodeProviderIDFromModel(model)
		content.Provider = map[string]openCodeProviderConfig{providerID: providerConfig}
	}
	if model != "" {
		content.Model = model
		if modelprovider.OpenCodeProviderIDFromModel(model) == modelprovider.ProviderOpenRouter {
			content.SmallModel = model
		}
	}
	addOpenCodeReasoningVariant(&content, model, agent.ReasoningEffort)
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	env["OPENCODE_CONFIG_CONTENT"] = string(data)
	return nil
}

func (m *Manager) prepareOpenCodeInstructionFile(ctx context.Context, env map[string]string, cwd, artifactSurface, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) (string, error) {
	prompt, err := m.systemPrompt(ctx, cwd, artifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return "", fmt.Errorf("build opencode instructions: %w", err)
	}
	if prompt == "" {
		return "", nil
	}
	path := filepath.Join(env["OPENCODE_CONFIG_DIR"], "jaz-instructions.md")
	if err := os.WriteFile(path, []byte(prompt+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write opencode instructions %s: %w", path, err)
	}
	return path, nil
}

func (m *Manager) openCodeProviderConfig(env map[string]string, model string) (openCodeProviderConfig, bool) {
	providerID, modelID := modelprovider.SplitProviderModel(model)
	if providerID == "" {
		return openCodeProviderConfig{}, false
	}
	cfg, configured := m.providers()[providerID]
	meta, builtIn := modelprovider.OpenCodeProviderByID(providerID)
	if !shouldWriteOpenCodeProviderConfig(cfg, meta, configured, builtIn) {
		return openCodeProviderConfig{}, false
	}
	baseURL := firstNonEmpty(cfg.BaseURL, meta.BaseURL)
	if baseURL == "" {
		return openCodeProviderConfig{}, false
	}
	key := openCodeConfiguredProviderEnv(providerID, cfg)
	if strings.TrimSpace(cfg.APIKey) != "" && key != "" && strings.TrimSpace(env[key]) == "" {
		env[key] = cfg.APIKey
	}
	result := openCodeProviderConfig{
		API:  baseURL,
		Name: firstNonEmpty(cfg.Label, meta.Label, providerID),
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
	if cfg.OpenCode {
		return strings.TrimSpace(firstNonEmpty(cfg.BaseURL, meta.BaseURL)) != ""
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

func openCodeConfiguredProviderEnv(id string, cfg modelprovider.ModelProviderConfig) string {
	return modelprovider.ConfiguredAPIKeyEnv(id, cfg)
}
