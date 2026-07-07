package acp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestDisconnectedAuthConfigPinsProfileAgents(t *testing.T) {
	for _, agent := range []string{AgentCodex, AgentClaude, AgentOpenCode} {
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

func TestRemoveOwnedCredentialAntigravityRemovesTokenAndKeyring(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set(antigravityKeyringService, antigravityKeyringAccount, "token"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	token := filepath.Join(dir, "antigravity-oauth-token")
	if err := os.WriteFile(token, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOwnedCredential(AgentAntigravity, token, filepath.Join(dir, "jaz-root")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(token); !os.IsNotExist(err) {
		t.Fatalf("token file still exists: %v", err)
	}
	if _, err := keyring.Get(antigravityKeyringService, antigravityKeyringAccount); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("keyring entry not removed: %v", err)
	}
}

func TestDisconnectedAuthConfigKeepsGrokExistingCLI(t *testing.T) {
	got := DisconnectedAuthConfig(AgentGrok, AgentAuthConfig{Mode: AuthModeExistingCLI})
	if got.Mode != AuthModeExistingCLI || got.Path != "" {
		t.Fatalf("grok disconnected auth = %#v, want existing CLI", got)
	}
}
