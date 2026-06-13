package acp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/runtimefiles"
)

const RefreshOwnerAgentCLI = "coding_agent_cli"

type AgentAuthStatus struct {
	Authenticated         bool
	Reason                string
	StoragePath           string
	LoginCommand          string
	LoginCommandAvailable bool
	LoginCommandReason    string
	RefreshOwner          string
}

func ProbeAgentAuth(name string, cfg AgentConfig, root string, env map[string]string) AgentAuthStatus {
	name = CanonicalAgentName(name)
	probeEnv := NewManager(nil, Config{Root: root, Env: env}, nil).probeEnv(name, cfg)
	status := agentLoginCommand(name, root)
	status.RefreshOwner = RefreshOwnerAgentCLI
	switch name {
	case AgentCodex:
		status.StoragePath = filepath.Join(probeEnv["CODEX_HOME"], "auth.json")
		status.Authenticated = codexAuthAvailable(probeEnv) || codexSourceAuthAvailable(cfg, env)
		status.Reason = codexAuthHint(probeEnv)
	case AgentClaude:
		status.StoragePath = filepath.Join(probeEnv["CLAUDE_CONFIG_DIR"], ".credentials.json")
		status.Authenticated = claudeAuthAvailable(probeEnv) || claudeSourceAuthAvailable(cfg, env)
		status.Reason = claudeAuthHint(probeEnv)
	case AgentGrok:
		status.StoragePath = filepath.Join(probeEnv["HOME"], ".grok", "auth.json")
		status.Authenticated = grokAuthAvailable(probeEnv) || grokSourceAuthAvailable(cfg, env) || strings.TrimSpace(probeEnv["XAI_API_KEY"]) != ""
		status.Reason = grokAuthHint(probeEnv)
	}
	if status.Authenticated {
		status.Reason = ""
	}
	return status
}

func codexSourceAuthAvailable(cfg AgentConfig, env map[string]string) bool {
	home := firstNonEmpty(cfg.Env["CODEX_HOME"], env["CODEX_HOME"], os.Getenv("CODEX_HOME"))
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		home = filepath.Join(userHome, ".codex")
	}
	return fileExists(filepath.Join(home, "auth.json"))
}

func claudeSourceAuthAvailable(cfg AgentConfig, env map[string]string) bool {
	sourceDir := firstNonEmpty(cfg.Env["CLAUDE_CONFIG_DIR"], env["CLAUDE_CONFIG_DIR"], os.Getenv("CLAUDE_CONFIG_DIR"))
	for _, candidate := range claudeCredentialCandidates(sourceDir) {
		if fileExists(candidate) {
			return true
		}
	}
	return false
}

func grokSourceAuthAvailable(cfg AgentConfig, env map[string]string) bool {
	home := firstNonEmpty(cfg.Env["HOME"], env["HOME"])
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		home = userHome
	}
	return fileExists(filepath.Join(home, ".grok", "auth.json"))
}

func agentLoginCommand(name, root string) AgentAuthStatus {
	layout := runtimefiles.New(root)
	switch CanonicalAgentName(name) {
	case AgentCodex:
		return loginCommand(map[string]string{"CODEX_HOME": layout.ACPCodexHome}, "codex", "login", "--device-auth")
	case AgentClaude:
		return loginCommand(map[string]string{"CLAUDE_CONFIG_DIR": layout.ACPClaudeConfig}, "claude", "auth", "login", "--claudeai")
	case AgentGrok:
		return loginCommand(map[string]string{"HOME": layout.ACPHome}, "grok", "login", "--device-auth")
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
