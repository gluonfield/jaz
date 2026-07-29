package acp

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestClaudeKeychainServiceUsesProfilePathHash(t *testing.T) {
	if got := claudeKeychainService("/var/lib/jaz/acp/claude"); got != "Claude Code-credentials-1ed971f6" {
		t.Fatalf("service = %q", got)
	}
}

func TestClaudeProfileAuthUnavailableWhenCredentialCleared(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"account-id"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"","refreshToken":"","expiresAt":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if claudeProfileAuthAvailable(configDir) {
		t.Fatal("blanked credential reported as an available login")
	}
}

func TestClaudeProfileAuthAvailableWithStoredCredential(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if !claudeProfileAuthAvailable(configDir) {
		t.Fatal("stored credential reported as missing")
	}
}

func TestClaudeProfileAuthFallsBackWhenCredentialStoreUnreadable(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin resolves the credential from the Keychain")
	}
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"account-id"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if !claudeProfileAuthAvailable(configDir) {
		t.Fatal("unreadable credential store must keep the profile's previous answer")
	}
}

func TestPrepareClaudeLoginClearsJazProfile(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "acp", "claude")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".claude.json", ".credentials.json", authFailureMarker} {
		if err := os.WriteFile(filepath.Join(configDir, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	invocation := AgentLoginInvocation{Env: map[string]string{"CLAUDE_CONFIG_DIR": configDir}}
	if err := PrepareAgentLoginInvocation(AgentClaude, AgentAuthConfig{Mode: AuthModeJazProfile}, root, invocation); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".claude.json", ".credentials.json", authFailureMarker} {
		if _, err := os.Stat(filepath.Join(configDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", name, err)
		}
	}
}

func TestPrepareClaudeLoginRejectsNonJazProfile(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	invocation := AgentLoginInvocation{Env: map[string]string{"CLAUDE_CONFIG_DIR": outside}}
	err := PrepareAgentLoginInvocation(AgentClaude, AgentAuthConfig{Mode: AuthModeJazProfile}, root, invocation)
	if err == nil {
		t.Fatal("expected non-Jaz profile to be rejected")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("non-Jaz profile was mutated: %v", err)
	}
}

func TestClaudeLoginUsesCanonicalJazProfile(t *testing.T) {
	input := AgentAuthConfig{Mode: AuthModeJazProfile, Path: "/tmp/not-jaz"}
	normalized, err := NormalizeAgentAuthConfig(AgentClaude, input)
	if err != nil {
		t.Fatal(err)
	}
	login, err := LoginAuthConfig(AgentClaude, input)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Path != "" || login.Mode != AuthModeJazProfile || login.Path != "" {
		t.Fatalf("normalized=%#v login=%#v", normalized, login)
	}
}

func TestRecordClaudeAuthFailureIgnoresUnrelatedErrors(t *testing.T) {
	root := t.TempDir()
	if err := recordClaudeAuthFailure(AgentAuthConfig{Mode: AuthModeJazProfile}, root, "network unavailable"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "acp", "claude", authFailureMarker)); !os.IsNotExist(err) {
		t.Fatalf("unrelated error recorded as auth failure: %v", err)
	}
}
