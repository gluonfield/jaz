package acp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessEnvPreservesProfileSkills(t *testing.T) {
	for _, agent := range []struct {
		name       string
		env        string
		defaultDir string
		jazDir     string
	}{
		{AgentCodex, "CODEX_HOME", ".codex", "codex-home"},
		{AgentClaude, "CLAUDE_CONFIG_DIR", ".claude", "claude"},
		{AgentKimi, "KIMI_CODE_HOME", ".kimi-code", "kimi"},
	} {
		for _, mode := range []string{AuthModeAuto, AuthModeExistingCLI, AuthModeJazProfile, "environment"} {
			t.Run(agent.name+"/"+mode, func(t *testing.T) {
				clearHostEnv(t)
				root := t.TempDir()
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("PATH", "/bin")
				t.Setenv("CLAUDE_CODE_EXECUTABLE", "claude")
				writeACPTestSkill(t, root, "alpha")
				writeACPTestSkill(t, root, "beta")
				if err := os.WriteFile(filepath.Join(root, "skills.sync.json"), []byte(`{"alpha":{"source":"jaz-skills","name":"alpha"}}`), 0o600); err != nil {
					t.Fatal(err)
				}

				profile := filepath.Join(home, agent.defaultDir)
				auth := AgentAuthConfig{Mode: mode}
				if mode == AuthModeExistingCLI {
					profile = filepath.Join(home, "custom")
					auth.Path = profile
				}
				if mode == "environment" {
					profile = filepath.Join(home, "from-env")
					auth.Mode = AuthModeAuto
					t.Setenv(agent.env, profile)
				}
				if mode == AuthModeJazProfile {
					profile = filepath.Join(root, "acp", agent.jazDir)
				}
				skill := filepath.Join(profile, "skills", "alpha", "SKILL.md")
				if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(skill, []byte("user-owned skill"), 0o644); err != nil {
					t.Fatal(err)
				}
				switch agent.name {
				case AgentCodex:
					writeCodexOAuth(t, profile)
				case AgentClaude:
					if err := os.WriteFile(filepath.Join(profile, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"test-account"}}`), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(profile, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"test-token"}}`), 0o600); err != nil {
						t.Fatal(err)
					}
				case AgentKimi:
					writeKimiCredential(t, profile, `{"access_token":"test-token"}`)
					writeKimiConfig(t, profile)
				}

				env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared(agent.name, AgentConfig{Auth: auth})
				if err != nil {
					t.Fatal(err)
				}
				if want := profile; agent.name == AgentClaude && mode == AuthModeAuto {
					if _, ok := env[agent.env]; ok {
						t.Fatal("pathless Claude login acquired a config directory")
					}
				} else if env[agent.env] != want {
					t.Fatalf("%s = %q, want %q", agent.env, env[agent.env], want)
				}
				if data, err := os.ReadFile(skill); err != nil || string(data) != "user-owned skill" {
					t.Fatalf("profile skill changed: %q, %v", data, err)
				}
				if _, err := os.Stat(filepath.Join(profile, "skills", "beta")); !os.IsNotExist(err) {
					t.Fatalf("Jaz skill copied to profile: %v", err)
				}
			})
		}
	}
}
