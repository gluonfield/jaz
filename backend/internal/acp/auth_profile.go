package acp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
)

const RefreshOwnerAgentCLI = "coding_agent_cli"

const (
	AuthKindOAuth  = "oauth"
	AuthKindAPIKey = "api_key"
)

type AgentAPIKeySpec struct {
	SourceEnv string `json:"source_env"`
	TargetEnv string `json:"target_env"`
}

type resolvedAgentAuth struct {
	Config        AgentAuthConfig
	StoragePath   string
	Source        string
	Evidence      string
	Kind          string
	Authenticated bool
	Reason        string
	APIKey        AgentAPIKeySpec
	APIKeySet     bool
	APIKeyValue   string
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
	path := strings.TrimSpace(auth.Path)
	if mode == AuthModeAuto {
		path = ""
	}
	if name == AgentGrok {
		if mode == AuthModeJazProfile {
			return AgentAuthConfig{}, fmt.Errorf("acp agent %q auth mode %q is not supported because Grok has no explicit profile path", name, mode)
		}
		if path != "" {
			return AgentAuthConfig{}, fmt.Errorf("acp agent %q auth path is not supported because Grok has no explicit profile path", name)
		}
	}
	return AgentAuthConfig{
		Mode: mode,
		Path: path,
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
	existing := expandAuthPath(firstNonEmpty(existingAuthPath(auth), cfg.Env["CODEX_HOME"], env["CODEX_HOME"], os.Getenv("CODEX_HOME"), defaultHomePath(".codex")))
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
	apiKeyConfigured := status.resolveAPIKey(AgentCodex, root, env)
	switch {
	case codexAuthFileAvailable(path):
		status.markAuthenticated("auth_json", AuthKindOAuth)
	case codexKeyringConfigured(path):
		status.markAuthenticated("keyring_config", AuthKindOAuth)
	case apiKeyConfigured:
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
	default:
		status.Reason = "Codex login at " + filepath.Join(path, "auth.json") + " or " + status.APIKey.SourceEnv
	}
	return status
}

func resolveClaudeAuth(auth AgentAuthConfig, cfg AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	layout := runtimefiles.New(root)
	existing := expandAuthPath(firstNonEmpty(existingAuthPath(auth), cfg.Env["CLAUDE_CONFIG_DIR"], env["CLAUDE_CONFIG_DIR"], os.Getenv("CLAUDE_CONFIG_DIR"), defaultHomePath(".claude")))
	jaz := layout.ACPClaudeConfig
	if auth.Mode == AuthModeJazProfile && strings.TrimSpace(auth.Path) != "" {
		jaz = expandAuthPath(auth.Path)
	}
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeJazProfile
		if claudeAuthFileAvailable(jaz) {
			mode = AuthModeJazProfile
		} else if claudeAccountAuthAvailable(cfg.Env) || claudeAccountAuthAvailable(env) || claudeAuthFileAvailable(existing) {
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
		StoragePath: claudeAuthPath(path),
		Source:      source,
	}
	apiKeyConfigured := status.resolveAPIKey(AgentClaude, root, env)
	switch {
	case claudeAccountAuthAvailable(cfg.Env) || claudeAccountAuthAvailable(env):
		status.markAuthenticated("env", AuthKindOAuth)
	case claudeAuthFileAvailable(path):
		status.markAuthenticated("claude_json", AuthKindOAuth)
	case apiKeyConfigured:
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
	default:
		status.Reason = "Claude login at " + claudeAuthPath(path) + " or " + status.APIKey.SourceEnv
	}
	return status
}

func resolveGrokAuth(auth AgentAuthConfig, _ AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	existing := defaultHomePath("")
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeExistingCLI
	}
	storagePath := ""
	if existing != "" {
		storagePath = filepath.Join(existing, ".grok", "auth.json")
	}
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode},
		StoragePath: storagePath,
		Source:      AuthModeExistingCLI,
	}
	apiKeyConfigured := status.resolveAPIKey(AgentGrok, root, env)
	switch {
	case existing != "" && grokAuthFileAvailable(existing):
		status.markAuthenticated("auth_json", AuthKindOAuth)
	case apiKeyConfigured:
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
	default:
		status.Reason = "Grok login at " + grokAuthPath(existing) + " or " + status.APIKey.SourceEnv
	}
	return status
}

func grokAuthPath(home string) string {
	if strings.TrimSpace(home) == "" {
		return "~/.grok/auth.json"
	}
	return filepath.Join(home, ".grok", "auth.json")
}

func existingAuthPath(auth AgentAuthConfig) string {
	if auth.Mode == AuthModeExistingCLI {
		return auth.Path
	}
	return ""
}

func (a *resolvedAgentAuth) resolveAPIKey(name, root string, env map[string]string) bool {
	a.APIKey, _ = resolveAgentAPIKeySpec(name)
	value, ok := explicitAgentAPIKey(name, root, env)
	a.APIKeySet = ok
	a.APIKeyValue = value
	return ok
}

func (a *resolvedAgentAuth) markAuthenticated(evidence, kind string) {
	a.Authenticated = true
	a.Evidence = evidence
	a.Kind = kind
}

func (a resolvedAgentAuth) APIKeyBinding() (string, string, bool) {
	if a.Kind != AuthKindAPIKey || strings.TrimSpace(a.APIKeyValue) == "" || strings.TrimSpace(a.APIKey.TargetEnv) == "" {
		return "", "", false
	}
	return a.APIKey.TargetEnv, a.APIKeyValue, true
}

func AgentAPIKey(name string) (AgentAPIKeySpec, bool) {
	return resolveAgentAPIKeySpec(name)
}

func resolveAgentAPIKeySpec(name string) (AgentAPIKeySpec, bool) {
	switch CanonicalAgentName(name) {
	case AgentCodex:
		return AgentAPIKeySpec{SourceEnv: "JAZ_ACP_CODEX_API_KEY", TargetEnv: "OPENAI_API_KEY"}, true
	case AgentClaude:
		return AgentAPIKeySpec{SourceEnv: "JAZ_ACP_CLAUDE_API_KEY", TargetEnv: "ANTHROPIC_API_KEY"}, true
	case AgentGrok:
		return AgentAPIKeySpec{SourceEnv: "JAZ_ACP_GROK_API_KEY", TargetEnv: "XAI_API_KEY"}, true
	default:
		return AgentAPIKeySpec{}, false
	}
}

func explicitAgentAPIKey(name, root string, env map[string]string) (string, bool) {
	spec, ok := resolveAgentAPIKeySpec(name)
	if !ok {
		return "", false
	}
	if value := strings.TrimSpace(env[spec.SourceEnv]); value != "" {
		return value, true
	}
	if value, ok := runtimeenv.Lookup(runtimeenv.Path(root), spec.SourceEnv); ok {
		return value, true
	}
	if value := strings.TrimSpace(os.Getenv(spec.SourceEnv)); value != "" {
		return value, true
	}
	return "", false
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
	return fileExists(claudeAuthPath(configDir)) || fileExists(filepath.Join(configDir, ".credentials.json"))
}

func claudeAuthPath(configDir string) string {
	return filepath.Join(configDir, ".claude.json")
}

func claudeAccountAuthAvailable(env map[string]string) bool {
	for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		if strings.TrimSpace(env[key]) != "" {
			return true
		}
	}
	return false
}

func grokAuthFileAvailable(home string) bool {
	return fileExists(filepath.Join(home, ".grok", "auth.json"))
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
