package acp

import (
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimefiles"
)

type AgentAuthStatus struct {
	Authenticated         bool
	Reason                string
	StoragePath           string
	AuthMode              string
	AuthPath              string
	AuthSource            string
	AuthEvidence          string
	AuthKind              string
	RecommendedAuth       AgentAuthConfig
	APIKey                AgentAPIKeySpec
	APIKeyConfigured      bool
	LoginCommand          string
	LoginCommandAvailable bool
	LoginCommandReason    string
	RefreshOwner          string
}

type AgentLoginInvocation struct {
	Env         map[string]string
	Executable  string
	Args        []string
	Display     string
	Available   bool
	Reason      string
	InheritHome bool
}

func ProbeAgentAuth(name string, cfg AgentConfig, root string, env map[string]string) AgentAuthStatus {
	return ProbeAgentAuthWithProviders(name, cfg, root, env, nil)
}

func ProbeAgentAuthWithProviders(name string, cfg AgentConfig, root string, env map[string]string, providers map[string]provider.ModelProviderConfig) AgentAuthStatus {
	name = CanonicalAgentName(name)
	if cfg.Local {
		return AgentAuthStatus{
			Authenticated: true,
			AuthKind:      AuthKindNone,
			AuthMode:      AuthModeAuto,
			RefreshOwner:  "jaz",
		}
	}
	probeEnv := NewManager(nil, Config{Root: root, Env: env, Providers: providers}, nil).probeEnv(name, cfg)
	resolved := resolveAgentAuthWithProviders(name, cfg, root, probeEnv, providers)
	status := agentLoginCommand(name, root, resolved.Config, loginBinDirs(cfg))
	status.RefreshOwner = RefreshOwnerAgentCLI
	status.StoragePath = resolved.StoragePath
	status.Authenticated = resolved.Authenticated
	status.Reason = resolved.Reason
	status.AuthMode = resolved.Config.Mode
	status.AuthPath = resolved.Config.Path
	status.AuthSource = resolved.Source
	status.AuthEvidence = resolved.Evidence
	status.AuthKind = resolved.Kind
	status.RecommendedAuth = resolved.Config
	status.APIKey = resolved.APIKey
	status.APIKeyConfigured = resolved.APIKeySet
	if status.Authenticated {
		status.Reason = ""
	}
	return status
}

func agentLoginCommand(name, root string, auth AgentAuthConfig, binDir string) AgentAuthStatus {
	invocation := AgentLoginInvocationFor(name, root, auth, binDir)
	return AgentAuthStatus{
		LoginCommand:          invocation.Display,
		LoginCommandAvailable: invocation.Available,
		LoginCommandReason:    invocation.Reason,
	}
}

// AgentLoginInvocationFor builds an agent's login command; binDir is searched before PATH.
func AgentLoginInvocationFor(name, root string, auth AgentAuthConfig, binDir string) AgentLoginInvocation {
	layout := runtimefiles.New(root)
	switch CanonicalAgentName(name) {
	case AgentCodex:
		home := firstNonEmpty(auth.Path, layout.ACPCodexHome)
		return loginInvocation(map[string]string{"CODEX_HOME": home}, false, binDir, "codex", "login", "--device-auth")
	case AgentClaude:
		configDir := firstNonEmpty(auth.Path, layout.ACPClaudeConfig)
		return loginInvocation(map[string]string{"CLAUDE_CONFIG_DIR": configDir}, false, binDir, "claude", "auth", "login", "--claudeai")
	case AgentGrok:
		return loginInvocation(nil, true, binDir, "grok", "login", "--device-auth")
	case AgentAntigravity:
		return loginInvocation(nil, true, binDir, "agy")
	default:
		return AgentLoginInvocation{}
	}
}

func loginBinDirs(cfg AgentConfig) string {
	if strings.TrimSpace(cfg.LoginBinDir) != "" {
		return cfg.LoginBinDir
	}
	return cfg.AdapterBinDir
}

func loginInvocation(env map[string]string, inheritHome bool, binDir, executable string, args ...string) AgentLoginInvocation {
	resolved, err := resolveLoginExecutable(binDir, executable)
	if err != nil {
		resolved = executable
	}
	parts := make([]string, 0, len(env)+1)
	for key, value := range env {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, key+"="+shellQuote(value))
		}
	}
	cmd := shellCommand(resolved, args...)
	if cmd != "" {
		parts = append(parts, cmd)
	}
	invocation := AgentLoginInvocation{
		Env:         env,
		Executable:  resolved,
		Args:        args,
		Display:     strings.Join(parts, " "),
		Available:   err == nil,
		InheritHome: inheritHome,
	}
	if err != nil {
		invocation.Reason = executable + " not found"
	}
	return invocation
}

func shellCommand(executable string, args ...string) string {
	parts := []string{shellQuote(executable)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}
