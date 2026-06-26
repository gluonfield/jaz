package acp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessEnvScrubsClaudeHostOAuthWhenAutoSelectsJazProfile(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	jazConfigDir := filepath.Join(root, "acp", "claude")
	if err := os.MkdirAll(jazConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jazConfigDir, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"account-id"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/bin")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "host-token")

	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{})

	if env["CLAUDE_CONFIG_DIR"] != jazConfigDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", env["CLAUDE_CONFIG_DIR"], jazConfigDir)
	}
	if env["CLAUDE_CODE_OAUTH_TOKEN"] != "" || env["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Fatalf("Claude OAuth token leaked into Jaz-profile subprocess env: %#v", env)
	}
}

func TestProcessEnvLeavesClaudeConfigDirUnsetForExistingCLIWithoutPath(t *testing.T) {
	clearHostEnv(t)
	t.Setenv("PATH", "/bin")
	root := t.TempDir()

	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeExistingCLI},
	})

	if got, ok := env["CLAUDE_CONFIG_DIR"]; ok {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want unset", got)
	}
	status := ProbeAgentAuth(AgentClaude, AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeExistingCLI},
	}, root, nil)
	if !status.Authenticated || status.AuthEvidence != "existing_cli" || status.AuthPath != "" {
		t.Fatalf("auth status = %#v, want pathless existing CLI", status)
	}
}

func TestProcessEnvUsesExplicitClaudeConfigDirForExistingCLI(t *testing.T) {
	clearHostEnv(t)
	t.Setenv("PATH", "/bin")
	root := t.TempDir()
	configDir := filepath.Join(t.TempDir(), "claude")

	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeExistingCLI, Path: configDir},
	})

	if env["CLAUDE_CONFIG_DIR"] != configDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", env["CLAUDE_CONFIG_DIR"], configDir)
	}
}

func TestProcessEnvUsesHostClaudeConfigDirForExistingCLI(t *testing.T) {
	clearHostEnv(t)
	t.Setenv("PATH", "/bin")
	root := t.TempDir()
	configDir := filepath.Join(t.TempDir(), "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeExistingCLI},
	})

	if env["CLAUDE_CONFIG_DIR"] != configDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want host %q", env["CLAUDE_CONFIG_DIR"], configDir)
	}
}
