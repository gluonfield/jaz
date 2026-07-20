package acp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareCodexLoginClearsNonOAuthCredential(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "acp", "codex-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	credential := filepath.Join(home, "auth.json")
	if err := os.WriteFile(credential, []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"key"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	invocation := AgentLoginInvocation{Env: map[string]string{"CODEX_HOME": home}}
	if err := PrepareAgentLoginInvocation(AgentCodex, AgentAuthConfig{Mode: AuthModeJazProfile}, root, invocation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(credential); !os.IsNotExist(err) {
		t.Fatalf("stale Codex credential remains: %v", err)
	}
}

func TestPrepareCodexLoginKeepsOAuthCredential(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "acp", "codex-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	writeCodexOAuth(t, home)
	credential := filepath.Join(home, "auth.json")
	want, err := os.ReadFile(credential)
	if err != nil {
		t.Fatal(err)
	}

	invocation := AgentLoginInvocation{Env: map[string]string{"CODEX_HOME": home}}
	if err := PrepareAgentLoginInvocation(AgentCodex, AgentAuthConfig{Mode: AuthModeJazProfile}, root, invocation); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(credential)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("valid Codex OAuth credential was changed")
	}
}

func TestCodexAPIKeyFileIsNotOAuth(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	home := filepath.Join(root, "acp", "codex-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"key"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	status := ProbeAgentAuth(AgentCodex, AgentConfig{Auth: AgentAuthConfig{Mode: AuthModeJazProfile}}, root, nil)
	if status.Authenticated {
		t.Fatalf("API-key credential reported as Codex OAuth: %#v", status)
	}
}

func TestCodexJazProfileCannotEscapeRuntimeRoot(t *testing.T) {
	auth, err := NormalizeAgentAuthConfig(AgentCodex, AgentAuthConfig{Mode: AuthModeJazProfile, Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if auth.Path != "" {
		t.Fatalf("normalized Codex profile path = %q, want Jaz-owned default", auth.Path)
	}
}

func TestPrepareCodexLoginRejectsNonJazProfile(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	invocation := AgentLoginInvocation{Env: map[string]string{"CODEX_HOME": outside}}
	err := PrepareAgentLoginInvocation(AgentCodex, AgentAuthConfig{Mode: AuthModeJazProfile}, root, invocation)
	if err == nil {
		t.Fatal("expected non-Jaz profile to be rejected")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("non-Jaz profile was mutated: %v", err)
	}
}

func writeCodexOAuth(t *testing.T, home string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
}
