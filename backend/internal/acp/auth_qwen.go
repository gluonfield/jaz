package acp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/processenv"
	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimefiles"
)

func qwenProvider(id string, providers map[string]modelprovider.ModelProviderConfig) (modelprovider.ModelProvider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	meta, ok := modelprovider.ModelProviderByID(id)
	if !ok {
		cfg, configured := providers[id]
		if !configured {
			return modelprovider.ModelProvider{}, false
		}
		meta = modelprovider.ApplyModelProviderConfig(modelprovider.ModelProvider{ID: id}, cfg)
	} else if cfg, configured := providers[id]; configured {
		meta = modelprovider.ApplyModelProviderConfig(meta, cfg)
	}
	return meta, meta.SupportsCapability(modelprovider.CapabilityChatCompletions)
}

func resolveQwenAuth(_ AgentAuthConfig, cfg AgentConfig, root string, env map[string]string, providers map[string]modelprovider.ModelProviderConfig) resolvedAgentAuth {
	home := runtimefiles.New(root).ACPQwenHome
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: AuthModeJazProfile, Path: home},
		StoragePath: home,
		Source:      strings.TrimSpace(cfg.ModelProvider),
	}
	meta, ok := qwenProvider(cfg.ModelProvider, providers)
	if !ok {
		status.Reason = "Select a Chat Completions provider for Qwen Code"
		return status
	}
	if !meta.RequiresAPIKey {
		status.markAuthenticated("no_api_key_required", AuthKindNone)
		return status
	}

	if meta.ID == modelprovider.ProviderQwenCodingPlan {
		if status.resolveAPIKey(AgentQwen, root, env) {
			status.APIKeyTargetAliases = []string{"OPENAI_API_KEY"}
			status.markAuthenticated("api_key_env", AuthKindAPIKey)
			return status
		}
	}
	keyEnv := strings.TrimSpace(meta.APIKeyEnv)
	value := strings.TrimSpace(providers[meta.ID].APIKey)
	if value == "" && keyEnv != "" {
		value = modelProviderKeyValue(root, env, keyEnv)
	}
	status.APIKey = AgentAPIKeySpec{SourceEnv: keyEnv, TargetEnv: keyEnv}
	status.APIKeySet = value != ""
	status.APIKeyValue = value
	status.APIKeyTargetAliases = []string{"OPENAI_API_KEY"}
	if value != "" {
		status.markAuthenticated(strings.ToLower(keyEnv)+"_env", AuthKindAPIKey)
		return status
	}
	if meta.ID == modelprovider.ProviderQwenCodingPlan {
		status.APIKey, _ = resolveAgentAPIKeySpec(AgentQwen)
		status.Reason = "Add your Qwen Coding Plan subscription key"
		return status
	}
	status.Reason = "Set " + firstNonEmpty(keyEnv, "the provider API key") + " in Settings > Model Providers"
	return status
}

func (m *Manager) prepareQwenProcessEnv(root string, cfg AgentConfig, env map[string]string, prepare bool) error {
	processenv.PreserveHost(env,
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"LANG",
		"LC_ALL",
		"LC_CTYPE",
		"LOGNAME",
		"NO_PROXY",
		"SHELL",
		"SSH_AUTH_SOCK",
		"USER",
	)
	providers := m.providers()
	auth := resolveAgentAuthWithProviders(AgentQwen, cfg, root, env, providers)
	for _, key := range modelProviderEnvNames(providers) {
		delete(env, key)
	}
	delete(env, "OPENAI_API_KEY")
	delete(env, "OPENAI_APIKEY")
	delete(env, "OPENAI_BASE_URL")
	delete(env, "OPENAI_MODEL")
	auth.BindAPIKeyEnv(env)
	home := runtimefiles.New(root).ACPQwenHome
	env["QWEN_HOME"] = home
	env["QWEN_RUNTIME_DIR"] = filepath.Join(home, "runtime")
	if meta, ok := qwenProvider(cfg.ModelProvider, providers); ok {
		env["OPENAI_BASE_URL"] = meta.BaseURL
	}
	if model := cfg.ProviderNativeModel(); model != "" {
		env["OPENAI_MODEL"] = model
	}
	if !prepare {
		return nil
	}
	if err := os.MkdirAll(env["QWEN_RUNTIME_DIR"], 0o700); err != nil {
		return fmt.Errorf("prepare qwen runtime %s: %w", env["QWEN_RUNTIME_DIR"], err)
	}
	m.installAgentSkills(AgentQwen, root, filepath.Join(home, "skills"))
	return nil
}

func qwenLaunchArgs(agent string, args []string, model, systemPrompt string) []string {
	if CanonicalAgentName(agent) != AgentQwen {
		return args
	}
	if model != "" && !hasFlag(args, "--model", "-m") {
		args = append(args, "--model", model)
	}
	if systemPrompt != "" && !hasFlag(args, "--append-system-prompt") {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	return args
}
