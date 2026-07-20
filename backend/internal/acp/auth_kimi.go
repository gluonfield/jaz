package acp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/wins/jaz/backend/internal/processenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
)

var errKimiModelNotConfigured = errors.New("Kimi sign-in did not finish configuring a model; sign in again")

type kimiConfig struct {
	DefaultModel    string                    `toml:"default_model"`
	DefaultProvider string                    `toml:"default_provider"`
	Providers       map[string]any            `toml:"providers"`
	Models          map[string]kimiModelAlias `toml:"models"`
}

type kimiModelAlias struct {
	Provider string `toml:"provider"`
}

func resolveKimiAuth(auth AgentAuthConfig, cfg AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	jaz := runtimefiles.New(root).ACPKimiHome
	existing := expandAuthPath(firstNonEmpty(existingAuthPath(auth), cfg.Env["KIMI_CODE_HOME"], env["KIMI_CODE_HOME"], os.Getenv("KIMI_CODE_HOME"), defaultHomePath(".kimi-code")))
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeJazProfile
		if !kimiAuthFileAvailable(jaz) && kimiAuthFileAvailable(existing) {
			mode = AuthModeExistingCLI
		}
	}
	home := jaz
	if mode == AuthModeExistingCLI {
		home = existing
	}
	storagePath := kimiAuthPath(home)
	source := mode
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode, Path: home},
		StoragePath: storagePath,
		Source:      source,
	}
	if kimiAuthFileAvailable(home) {
		status.markAuthenticated("oauth_json", AuthKindOAuth)
	} else {
		status.Reason = "Kimi login at " + storagePath
	}
	return status
}

func kimiAuthFileAvailable(home string) bool {
	data, err := os.ReadFile(kimiAuthPath(home))
	if err != nil {
		return false
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	return json.Unmarshal(data, &token) == nil && token.AccessToken != ""
}

func kimiAuthPath(home string) string {
	return filepath.Join(home, "credentials", "kimi-code.json")
}

func kimiModelConfigReady(home string) error {
	data, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		return errKimiModelNotConfigured
	}
	var cfg kimiConfig
	if toml.Unmarshal(data, &cfg) != nil {
		return errKimiModelNotConfigured
	}
	modelID := strings.TrimSpace(cfg.DefaultModel)
	model, ok := cfg.Models[modelID]
	if modelID == "" || !ok {
		return errKimiModelNotConfigured
	}
	providerID := strings.TrimSpace(model.Provider)
	if providerID == "" {
		providerID = strings.TrimSpace(cfg.DefaultProvider)
	}
	if _, ok := cfg.Providers[providerID]; providerID == "" || !ok {
		return errKimiModelNotConfigured
	}
	return nil
}

func (m *Manager) prepareKimiProcessEnv(root string, cfg AgentConfig, env map[string]string, prepare bool) error {
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
	auth := resolveAgentAuthWithProviders(AgentKimi, cfg, root, env, m.providers())
	env["KIMI_CODE_HOME"] = auth.Config.Path
	if !prepare {
		return nil
	}
	if auth.Authenticated {
		if err := kimiModelConfigReady(auth.Config.Path); err != nil {
			return err
		}
	}
	m.installAgentSkills(AgentKimi, root, filepath.Join(env["KIMI_CODE_HOME"], "skills"))
	return nil
}
