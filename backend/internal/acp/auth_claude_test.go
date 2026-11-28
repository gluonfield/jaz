package acp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type fakeExitError int

func (e fakeExitError) Error() string { return "exit error" }
func (e fakeExitError) ExitCode() int { return int(e) }

func stubClaudeSecurity(t *testing.T, goos string, run func(ctx context.Context, args ...string) error) {
	t.Helper()
	prevGOOS := claudeKeychainGOOS
	prevRun := claudeSecurityRun
	claudeKeychainGOOS = func() string { return goos }
	claudeSecurityRun = run
	t.Cleanup(func() {
		claudeKeychainGOOS = prevGOOS
		claudeSecurityRun = prevRun
	})
}

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
	stubClaudeSecurity(t, "darwin", func(context.Context, ...string) error {
		return errors.New("keychain temporarily unavailable")
	})
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"account-id"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if !claudeProfileAuthAvailable(configDir) {
		t.Fatal("unreadable credential store must keep the profile's previous answer")
	}
}

func TestClaudePassiveProbeChecksKeychainMetadataWithoutSecretFlag(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"account-id"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	stubClaudeSecurity(t, "darwin", func(_ context.Context, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})
	if !claudeProfileAuthAvailable(configDir) {
		t.Fatal("expected existing Keychain metadata to report available")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 security call, got %d", len(calls))
	}
	want := []string{"find-generic-password", "-s", claudeKeychainService(configDir)}
	if !slices.Equal(calls[0], want) {
		t.Fatalf("security args = %q, want %q", calls[0], want)
	}
	if slices.Contains(calls[0], "-w") {
		t.Fatalf("passive Claude probe must never pass -w: %q", calls[0])
	}

	stubClaudeSecurity(t, "darwin", func(_ context.Context, args ...string) error {
		if slices.Contains(args, "-w") {
			t.Fatalf("passive Claude probe must never pass -w: %q", args)
		}
		return fakeExitError(macOSSecurityItemNotFound)
	})
	if claudeProfileAuthAvailable(configDir) {
		t.Fatal("missing Keychain item must report unavailable even when .claude.json remains")
	}
}

func TestClaudePassiveProbeSkipsKeychainWhenCredentialsFileExists(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stubClaudeSecurity(t, "darwin", func(context.Context, ...string) error {
		t.Fatal("must not call security when .credentials.json is present")
		return nil
	})
	if !claudeProfileAuthAvailable(configDir) {
		t.Fatal("stored .credentials.json reported as missing")
	}
}

func TestPrepareClaudeLoginClearsJazProfile(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "acp", "claude")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".claude.json", ".credentials.json", ".jaz-auth-failed"} {
		if err := os.WriteFile(filepath.Join(configDir, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	invocation := AgentLoginInvocation{Env: map[string]string{"CLAUDE_CONFIG_DIR": configDir}}
	if err := PrepareAgentLoginInvocation(AgentClaude, AgentAuthConfig{Mode: AuthModeJazProfile}, root, invocation); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".claude.json", ".credentials.json", ".jaz-auth-failed"} {
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
