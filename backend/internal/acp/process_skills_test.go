package acp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessEnvPreparedInstallsCodexSkills(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	writeACPTestSkill(t, root, "alpha")

	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("codex", AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeJazProfile},
	})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(env["CODEX_HOME"], "skills", "alpha", "SKILL.md")
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "Alpha skill") {
		t.Fatalf("codex skill copy = %q, %v", data, err)
	}
}

func TestProcessEnvPreparedIgnoresCodexSkillInstallFailure(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	writeACPTestSkill(t, root, "alpha")
	profile := filepath.Join(root, "acp", "codex-home")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "skills"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("codex", AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeJazProfile},
	})
	if err != nil {
		t.Fatal(err)
	}
	if env["CODEX_HOME"] != profile {
		t.Fatalf("CODEX_HOME = %q, want %q", env["CODEX_HOME"], profile)
	}
}
