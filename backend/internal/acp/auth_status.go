package acp

import (
	"strings"

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

func ProbeAgentAuth(name string, cfg AgentConfig, root string, env map[string]string) AgentAuthStatus {
	name = CanonicalAgentName(name)
	probeEnv := NewManager(nil, Config{Root: root, Env: env}, nil).probeEnv(name, cfg)
	resolved := resolveAgentAuth(name, cfg, root, probeEnv)
	status := agentLoginCommand(name, root, resolved.Config)
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

func agentLoginCommand(name, root string, auth AgentAuthConfig) AgentAuthStatus {
	layout := runtimefiles.New(root)
	switch CanonicalAgentName(name) {
	case AgentCodex:
		home := firstNonEmpty(auth.Path, layout.ACPCodexHome)
		return loginCommand(map[string]string{"CODEX_HOME": home}, "codex", "login", "--device-auth")
	case AgentClaude:
		configDir := firstNonEmpty(auth.Path, layout.ACPClaudeConfig)
		return loginCommand(map[string]string{"CLAUDE_CONFIG_DIR": configDir}, "claude", "auth", "login", "--claudeai")
	case AgentGrok:
		return loginCommand(nil, "grok", "login", "--device-auth")
	default:
		return AgentAuthStatus{}
	}
}

func loginCommand(env map[string]string, executable string, args ...string) AgentAuthStatus {
	resolved, err := ResolveExecutable(executable)
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
	status := AgentAuthStatus{
		LoginCommand:          strings.Join(parts, " "),
		LoginCommandAvailable: err == nil,
	}
	if err != nil {
		status.LoginCommandReason = executable + " not found"
	}
	return status
}

func shellCommand(executable string, args ...string) string {
	parts := []string{shellQuote(executable)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}
