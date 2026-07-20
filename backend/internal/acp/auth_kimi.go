package acp

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/processenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
)

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
	m.installAgentSkills(AgentKimi, root, filepath.Join(env["KIMI_CODE_HOME"], "skills"))
	return nil
}
