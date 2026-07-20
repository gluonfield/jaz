package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/promptmodule"
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
