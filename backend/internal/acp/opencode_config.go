package acp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
)

const openCodeOpenAICompatibleNPM = "@ai-sdk/openai-compatible"

type openCodeConfigContent struct {
	Instructions []string                          `json:"instructions,omitempty"`
	Provider     map[string]openCodeProviderConfig `json:"provider,omitempty"`
}

type openCodeProviderConfig struct {
	API    string                         `json:"api,omitempty"`
	Name   string                         `json:"name,omitempty"`
	Env    []string                       `json:"env,omitempty"`
	NPM    string                         `json:"npm,omitempty"`
	Models map[string]openCodeModelConfig `json:"models,omitempty"`
}

type openCodeModelConfig struct {
	ID string `json:"id,omitempty"`
}

func (m *Manager) loadOpenCodeProviderEnv(env map[string]string, root string) {
	for _, key := range openCodeProviderEnvNames(m.cfg.Providers) {
		loadRuntimeEnvKey(env, root, key)
	}
	for id, cfg := range m.cfg.Providers {
		if strings.TrimSpace(cfg.APIKey) == "" {
			continue
		}
		key := openCodeConfiguredProviderEnv(id, cfg)
		if key != "" && strings.TrimSpace(env[key]) == "" {
			env[key] = cfg.APIKey
		}
	}
}

func openCodeProviderEnvNames(configs map[string]modelprovider.ModelProviderConfig) []string {
	keys := map[string]struct{}{}
	for _, provider := range modelprovider.ModelProviders() {
		if provider.OpenCode && strings.TrimSpace(provider.APIKeyEnv) != "" {
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

func (m *Manager) prepareOpenCodeConfig(env map[string]string, agent AgentConfig) error {
	if strings.TrimSpace(env["OPENCODE_CONFIG_CONTENT"]) != "" {
		return nil
	}
	content := openCodeConfigContent{}
	if instruction, err := m.prepareOpenCodeInstructionFile(env); err != nil {
		return err
	} else if instruction != "" {
		content.Instructions = []string{instruction}
	}
	if providerConfig, ok := m.openCodeProviderConfig(env, agent.Model); ok {
		providerID := openCodeProviderID(agent.Model)
		content.Provider = map[string]openCodeProviderConfig{providerID: providerConfig}
	}
	if len(content.Instructions) == 0 && len(content.Provider) == 0 {
		return nil
	}
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	env["OPENCODE_CONFIG_CONTENT"] = string(data)
	return nil
}

func (m *Manager) prepareOpenCodeInstructionFile(env map[string]string) (string, error) {
	if m.cfg.SystemPrompt == nil {
		return "", nil
	}
	prompt, err := m.cfg.SystemPrompt.ACPPrompt()
	if err != nil {
		return "", fmt.Errorf("build opencode instructions: %w", err)
	}
	prompt = strings.TrimSpace(prompt)
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
	cfg, configured := m.cfg.Providers[providerID]
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
	if key := strings.TrimSpace(cfg.APIKeyEnv); key != "" {
		return key
	}
	if meta, ok := modelprovider.OpenCodeProviderByID(id); ok && strings.TrimSpace(meta.APIKeyEnv) != "" {
		return meta.APIKeyEnv
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return ""
	}
	return "JAZ_PROVIDER_" + envKeyName(id) + "_API_KEY"
}

func envKeyName(id string) string {
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
