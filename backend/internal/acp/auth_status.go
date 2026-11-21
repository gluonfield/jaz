package acp

import (
	"path/filepath"
	"runtime"
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
	ManualDisconnect      string
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
	// UsePTY runs the login on a pseudo-terminal: agy drains piped stdin as
	// prompt input and would stall before ever starting its OAuth flow.
	UsePTY bool
	// Cwd overrides the login working directory. agy adopts the cwd as its
	// workspace and indexes it; sign-in should not hand it the Jaz root.
	Cwd string
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
	return agentAuthStatus(name, cfg, root, probeEnv, resolved)
}

func agentAuthStatus(name string, cfg AgentConfig, root string, env map[string]string, resolved resolvedAgentAuth) AgentAuthStatus {
	var status AgentAuthStatus
	if CanonicalAgentName(name) == AgentMicrosoftCopilot {
		status = microsoftCopilotLoginCommand(cfg)
		status.RefreshOwner = RefreshOwnerJazOAuth
	} else {
		status = agentLoginCommand(name, root, resolved.Config, loginBinDirs(cfg), env)
		status.RefreshOwner = RefreshOwnerAgentCLI
	}
	status.ManualDisconnect = agentManualDisconnect(name, cfg)
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

func agentLoginCommand(name, root string, auth AgentAuthConfig, binDir string, env map[string]string) AgentAuthStatus {
	invocation := AgentLoginInvocationFor(name, root, auth, binDir, env)
	return AgentAuthStatus{
		LoginCommand:          invocation.Display,
		LoginCommandAvailable: invocation.Available,
		LoginCommandReason:    invocation.Reason,
	}
}

// AgentLoginInvocationFor builds an agent's login command; binDir is searched before PATH.
func AgentLoginInvocationFor(name, root string, auth AgentAuthConfig, binDir string, env map[string]string) AgentLoginInvocation {
	layout := runtimefiles.New(root)
	switch CanonicalAgentName(name) {
	case AgentCodex:
		home := firstNonEmpty(auth.Path, layout.ACPCodexHome)
		return loginInvocation(map[string]string{"CODEX_HOME": home}, false, binDir, "codex", "login", "--device-auth")
	case AgentClaude:
		configDir := firstNonEmpty(auth.Path, layout.ACPClaudeConfig)
		loginEnv := map[string]string{"CLAUDE_CONFIG_DIR": configDir}
		preserveRuntimeHome(loginEnv)
		if auth.Mode == AuthModeJazProfile {
			loginEnv["ANTHROPIC_CONFIG_DIR"] = claudeSDKConfigDir(configDir, env)
			loginEnv["ANTHROPIC_PROFILE"] = env["ANTHROPIC_PROFILE"]
		}
		return loginInvocation(loginEnv, false, binDir, "claude", "auth", "login", "--claudeai")
	case AgentKimi:
		home := firstNonEmpty(auth.Path, layout.ACPKimiHome)
		return loginInvocation(map[string]string{"KIMI_CODE_HOME": home}, false, binDir, "kimi", "login", "--region", "global")
	case AgentGrok:
		return loginInvocation(nil, true, binDir, "grok", "login", "--device-auth")
	case AgentCopilot:
		probeEnv := NewManager(nil, Config{Root: root, Env: env}, nil).probeEnv(name, AgentConfig{LoginBinDir: binDir})
		invocation := loginInvocation(probeEnv, true, binDir, "copilot", "login", "--device-code")
		invocation.Executable, invocation.Args = launchCommand(invocation.Executable, invocation.Args)
		return invocation
	case AgentMuse:
		probeEnv := NewManager(nil, Config{Root: root, Env: env}, nil).probeEnv(name, AgentConfig{LoginBinDir: binDir})
		return museLoginInvocation(probeEnv)
	case AgentAntigravity:
		executable, err := antigravityExecutable(binDir)
		if err != nil {
			executable = "antigravity"
		}
		invocation := loginInvocation(nil, true, "", executable, "acp", "--login")
		invocation.UsePTY = runtime.GOOS != "windows"
		invocation.Cwd = filepath.Join(root, "acp", "agy-login-workspace")
		return invocation
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
