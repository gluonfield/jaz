package acp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/wins/jaz/backend/internal/runtimefiles"
)

const RefreshOwnerAgentCLI = "coding_agent_cli"

type resolvedAgentAuth struct {
	Config        AgentAuthConfig
	StoragePath   string
	Source        string
	Evidence      string
	Authenticated bool
	Reason        string
}

func NormalizeAgentAuthConfig(name string, auth AgentAuthConfig) (AgentAuthConfig, error) {
	name = CanonicalAgentName(name)
	mode := strings.TrimSpace(auth.Mode)
	if mode == "" {
		mode = AuthModeAuto
	}
	switch mode {
	case AuthModeAuto, AuthModeExistingCLI, AuthModeJazProfile:
	default:
		return AgentAuthConfig{}, fmt.Errorf("acp agent %q auth mode %q is not supported", name, mode)
	}
	return AgentAuthConfig{
		Mode: mode,
		Path: strings.TrimSpace(auth.Path),
	}, nil
}

func resolveAgentAuth(name string, cfg AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	name = CanonicalAgentName(name)
	auth, err := NormalizeAgentAuthConfig(name, cfg.Auth)
	if err != nil {
		auth = AgentAuthConfig{Mode: AuthModeAuto}
	}
	switch name {
	case AgentCodex:
		return resolveCodexAuth(auth, cfg, root, env)
	case AgentClaude:
		return resolveClaudeAuth(auth, cfg, root, env)
	case AgentGrok:
		return resolveGrokAuth(auth, cfg, root, env)
	default:
		return resolvedAgentAuth{Config: auth}
	}
}

func resolveCodexAuth(auth AgentAuthConfig, cfg AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	layout := runtimefiles.New(root)
	existing := expandAuthPath(firstNonEmpty(auth.Path, cfg.Env["CODEX_HOME"], env["CODEX_HOME"], os.Getenv("CODEX_HOME"), defaultHomePath(".codex")))
	jaz := layout.ACPCodexHome
	if auth.Mode == AuthModeJazProfile && strings.TrimSpace(auth.Path) != "" {
		jaz = expandAuthPath(auth.Path)
	}
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeJazProfile
		if codexAuthFileAvailable(jaz) {
			mode = AuthModeJazProfile
		} else if codexAuthFileAvailable(existing) || codexKeyringConfigured(existing) {
			mode = AuthModeExistingCLI
		}
	}
	path := jaz
	source := AuthModeJazProfile
	if mode == AuthModeExistingCLI {
		path = existing
		source = AuthModeExistingCLI
	}
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode, Path: path},
		StoragePath: filepath.Join(path, "auth.json"),
		Source:      source,
	}
	switch {
	case codexAuthFileAvailable(path):
		status.Authenticated = true
		status.Evidence = "auth_json"
	case codexKeyringConfigured(path):
		status.Authenticated = true
		status.Evidence = "keyring_config"
	default:
		status.Reason = "Codex OAuth login at " + filepath.Join(path, "auth.json")
	}
	return status
}

func resolveClaudeAuth(auth AgentAuthConfig, cfg AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	layout := runtimefiles.New(root)
	existing := expandAuthPath(firstNonEmpty(auth.Path, cfg.Env["CLAUDE_CONFIG_DIR"], env["CLAUDE_CONFIG_DIR"], os.Getenv("CLAUDE_CONFIG_DIR"), defaultHomePath(".claude")))
	jaz := layout.ACPClaudeConfig
	if auth.Mode == AuthModeJazProfile && strings.TrimSpace(auth.Path) != "" {
		jaz = expandAuthPath(auth.Path)
	}
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeJazProfile
		if claudeAuthFileAvailable(jaz) {
			mode = AuthModeJazProfile
		} else if claudeEnvAuthAvailable(cfg.Env) || claudeEnvAuthAvailable(env) || claudeAuthFileAvailable(existing) {
			mode = AuthModeExistingCLI
		}
	}
	path := jaz
	source := AuthModeJazProfile
	if mode == AuthModeExistingCLI {
		path = existing
		source = AuthModeExistingCLI
	}
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode, Path: path},
		StoragePath: filepath.Join(path, ".credentials.json"),
		Source:      source,
	}
	switch {
	case claudeEnvAuthAvailable(cfg.Env) || claudeEnvAuthAvailable(env):
		status.Authenticated = true
		status.Evidence = "env"
	case claudeAuthFileAvailable(path):
		status.Authenticated = true
		status.Evidence = "credentials_json"
	default:
		status.Reason = "Claude login at " + filepath.Join(path, ".credentials.json")
	}
	return status
}

func resolveGrokAuth(auth AgentAuthConfig, cfg AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	layout := runtimefiles.New(root)
	existing := expandAuthPath(firstNonEmpty(auth.Path, cfg.Env["HOME"], env["HOME"], os.Getenv("HOME"), defaultHomePath("")))
	jaz := layout.ACPHome
	if auth.Mode == AuthModeJazProfile && strings.TrimSpace(auth.Path) != "" {
		jaz = expandAuthPath(auth.Path)
	}
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeJazProfile
		if grokAuthFileAvailable(jaz) {
			mode = AuthModeJazProfile
		} else if grokEnvAuthAvailable(cfg.Env) || grokEnvAuthAvailable(env) || grokAuthFileAvailable(existing) {
			mode = AuthModeExistingCLI
		}
	}
	path := jaz
	source := AuthModeJazProfile
	if mode == AuthModeExistingCLI {
		path = existing
		source = AuthModeExistingCLI
	}
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode, Path: path},
		StoragePath: filepath.Join(path, ".grok", "auth.json"),
		Source:      source,
	}
	switch {
	case grokEnvAuthAvailable(cfg.Env) || grokEnvAuthAvailable(env):
		status.Authenticated = true
		status.Evidence = "env"
	case grokAuthFileAvailable(path):
		status.Authenticated = true
		status.Evidence = "auth_json"
	default:
		status.Reason = "Grok login at " + filepath.Join(path, ".grok", "auth.json") + " or XAI_API_KEY"
	}
	return status
}

func codexAuthFileAvailable(home string) bool {
	return fileExists(filepath.Join(home, "auth.json"))
}

func codexKeyringConfigured(home string) bool {
	var config struct {
		CredentialsStore string `toml:"cli_auth_credentials_store"`
	}
	data, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		return false
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		return false
	}
	store := strings.ToLower(strings.TrimSpace(config.CredentialsStore))
	return store == "keyring" || store == "auto"
}

func claudeAuthFileAvailable(configDir string) bool {
	return fileExists(filepath.Join(configDir, ".credentials.json"))
}

func claudeEnvAuthAvailable(env map[string]string) bool {
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		if strings.TrimSpace(env[key]) != "" {
			return true
		}
	}
	return false
}

func grokAuthFileAvailable(home string) bool {
	return fileExists(filepath.Join(home, ".grok", "auth.json"))
}

func grokEnvAuthAvailable(env map[string]string) bool {
	return strings.TrimSpace(env["XAI_API_KEY"]) != "" || strings.TrimSpace(env["XAI_APIKEY"]) != ""
}

func defaultHomePath(child string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if strings.TrimSpace(child) == "" {
		return home
	}
	return filepath.Join(home, child)
}

func expandAuthPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "~" {
		return defaultHomePath("")
	}
	if strings.HasPrefix(path, "~/") {
		if home := defaultHomePath(""); home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
