package acp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisconnectedAuthConfigPinsProfileAgents(t *testing.T) {
	for _, agent := range []string{AgentCodex, AgentClaude, AgentKimi, AgentOpenCode} {
		if got := DisconnectedAuthConfig(agent, AgentAuthConfig{Mode: AuthModeExistingCLI}); got.Mode != AuthModeJazProfile || got.Path != "" {
			t.Fatalf("%s disconnected auth = %#v, want Jaz profile", agent, got)
		}
	}
}

func TestDisconnectedAuthConfigResetsAntigravityToAuto(t *testing.T) {
	if got := DisconnectedAuthConfig(AgentAntigravity, AgentAuthConfig{Mode: AuthModeExistingCLI}); got.Mode != AuthModeAuto || got.Path != "" {
		t.Fatalf("antigravity disconnected auth = %#v, want auto", got)
	}
}

func TestNormalizeAgentAuthAllowsAntigravityExistingCLI(t *testing.T) {
	got, err := NormalizeAgentAuthConfig(AgentAntigravity, AgentAuthConfig{Mode: AuthModeExistingCLI})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != AuthModeExistingCLI {
		t.Fatalf("mode = %q, want existing CLI", got.Mode)
	}
}

func TestNormalizeAgentAuthMapsAntigravityJazProfileToAuto(t *testing.T) {
	got, err := NormalizeAgentAuthConfig(AgentAntigravity, AgentAuthConfig{Mode: AuthModeJazProfile, Path: "/tmp/profile"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != AuthModeAuto || got.Path != "" {
		t.Fatalf("auth = %#v, want auto", got)
	}
}

func TestRemoveOwnedCredentialSkipsDirectoryStoragePath(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "acp", "opencode")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	instructions := filepath.Join(configDir, "jaz-instructions.md")
	if err := os.WriteFile(instructions, []byte("instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOwnedCredential(AgentOpenCode, configDir, root); err != nil {
		t.Fatalf("directory storage path must be a no-op: %v", err)
	}
	if _, err := os.Stat(instructions); err != nil {
		t.Fatalf("config dir contents removed: %v", err)
	}
}

func TestPathUnderRootRejectsSymlinkedDir(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "jaz-root")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "acp")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if pathUnderRoot(filepath.Join(link, "auth.json"), root) {
		t.Fatal("symlinked dir escaping root must not count as Jaz-owned")
	}
	inside := filepath.Join(root, "profile")
	if err := os.MkdirAll(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	if !pathUnderRoot(filepath.Join(inside, "auth.json"), root) {
		t.Fatal("real dir under root must count as Jaz-owned")
	}
}

func TestDisconnectedAuthConfigKeepsGrokExistingCLI(t *testing.T) {
	got := DisconnectedAuthConfig(AgentGrok, AgentAuthConfig{Mode: AuthModeExistingCLI})
	if got.Mode != AuthModeExistingCLI || got.Path != "" {
		t.Fatalf("grok disconnected auth = %#v, want existing CLI", got)
	}
}
