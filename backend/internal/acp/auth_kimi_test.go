package acp

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestKimiBuiltinAgentUsesManagedACP(t *testing.T) {
	cfg := BuiltinAgents()[AgentKimi]
	if cfg.Command != "" || cfg.ManagedAdapter != "kimi" || !reflect.DeepEqual(cfg.ManagedAdapterArgs, []string{"acp"}) {
		t.Fatalf("Kimi config = %#v, want managed kimi acp", cfg)
	}
	if cfg.Model != "" || cfg.ProviderMode != "" {
		t.Fatalf("Kimi must use its native account defaults: %#v", cfg)
	}
}

func TestProbeAgentAuthDetectsKimiOAuthProfile(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	home := filepath.Join(root, "acp", "kimi")
	writeKimiCredential(t, home, `{"access_token":"token"}`)

	status := ProbeAgentAuth(AgentKimi, AgentConfig{}, root, nil)
	if !status.Authenticated || status.AuthKind != AuthKindOAuth || status.AuthEvidence != "oauth_json" {
		t.Fatalf("Kimi auth status = %#v", status)
	}
	if status.AuthSource != AuthModeJazProfile || status.AuthPath != home || status.StoragePath != kimiAuthPath(home) {
		t.Fatalf("Kimi auth profile = %#v", status)
	}
}

func TestProbeAgentAuthUsesExistingKimiProfile(t *testing.T) {
	clearHostEnv(t)
	existing := t.TempDir()
	writeKimiCredential(t, existing, `{"access_token":"token"}`)
	root := t.TempDir()

	status := ProbeAgentAuth(AgentKimi, AgentConfig{Env: map[string]string{"KIMI_CODE_HOME": existing}}, root, nil)
	if !status.Authenticated || status.AuthSource != AuthModeExistingCLI || status.AuthPath != existing {
		t.Fatalf("Kimi existing auth = %#v", status)
	}
	if _, err := os.Stat(filepath.Join(root, "acp", "kimi")); !os.IsNotExist(err) {
		t.Fatalf("auth probe created Jaz profile: %v", err)
	}
}

func TestProbeAgentAuthPrefersJazKimiProfile(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	jaz := filepath.Join(root, "acp", "kimi")
	existing := t.TempDir()
	writeKimiCredential(t, jaz, `{"access_token":"jaz"}`)
	writeKimiCredential(t, existing, `{"access_token":"existing"}`)

	status := ProbeAgentAuth(AgentKimi, AgentConfig{Env: map[string]string{"KIMI_CODE_HOME": existing}}, root, nil)
	if !status.Authenticated || status.AuthSource != AuthModeJazProfile || status.AuthPath != jaz {
		t.Fatalf("Kimi preferred auth = %#v", status)
	}
}

func TestKimiCredentialRequiresAccessToken(t *testing.T) {
	for _, content := range []string{`{}`, `{"access_token":""}`, `{not-json`} {
		home := t.TempDir()
		writeKimiCredential(t, home, content)
		if kimiAuthFileAvailable(home) {
			t.Fatalf("credential %q was accepted", content)
		}
	}
}

func TestProcessEnvPreparesIsolatedKimiProfile(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	writeACPTestSkill(t, root, "alpha")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/ssh-agent.sock")
	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared(AgentKimi, AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeJazProfile},
	})
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "acp", "kimi")
	if env["KIMI_CODE_HOME"] != home {
		t.Fatalf("KIMI_CODE_HOME = %q, want %q", env["KIMI_CODE_HOME"], home)
	}
	if env["SSH_AUTH_SOCK"] != "/tmp/ssh-agent.sock" {
		t.Fatalf("SSH_AUTH_SOCK = %q, want host agent", env["SSH_AUTH_SOCK"])
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatalf("Kimi skill not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("Kimi profile must not carry a prompt authority shim: %v", err)
	}
}

func TestKimiJazProfileCannotEscapeRuntimeRoot(t *testing.T) {
	auth, err := NormalizeAgentAuthConfig(AgentKimi, AgentAuthConfig{Mode: AuthModeJazProfile, Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if auth.Path != "" {
		t.Fatalf("normalized Kimi profile path = %q, want Jaz-owned default", auth.Path)
	}
}

func TestNormalizeAgentAuthAcceptsExistingKimiProfile(t *testing.T) {
	path := t.TempDir()
	auth, err := NormalizeAgentAuthConfig(AgentKimi, AgentAuthConfig{Mode: AuthModeExistingCLI, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if auth.Mode != AuthModeExistingCLI || auth.Path != path {
		t.Fatalf("normalized Kimi auth = %#v", auth)
	}
}

func TestProcessEnvUsesExistingKimiProfile(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	existing := t.TempDir()
	writeKimiCredential(t, existing, `{"access_token":"token"}`)

	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared(AgentKimi, AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeExistingCLI, Path: existing},
	})
	if err != nil {
		t.Fatal(err)
	}
	if env["KIMI_CODE_HOME"] != existing {
		t.Fatalf("KIMI_CODE_HOME = %q, want %q", env["KIMI_CODE_HOME"], existing)
	}
}

func TestRemoveOwnedKimiCredentialKeepsProfileState(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "acp", "kimi")
	writeKimiCredential(t, home, `{"access_token":"token"}`)
	config := filepath.Join(home, "config.toml")
	if err := os.WriteFile(config, []byte("[ui]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOwnedCredential(AgentKimi, kimiAuthPath(home), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(kimiAuthPath(home)); !os.IsNotExist(err) {
		t.Fatalf("Kimi credential remains: %v", err)
	}
	if _, err := os.Stat(config); err != nil {
		t.Fatalf("Kimi profile state was removed: %v", err)
	}
}

func writeKimiCredential(t *testing.T, home, content string) {
	t.Helper()
	path := kimiAuthPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
