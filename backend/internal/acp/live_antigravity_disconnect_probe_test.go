//go:build acpprobe && darwin

package acp

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestLiveAntigravityDisconnectProbe runs the production credential removal
// against the real keychain and agy CLI, then restores the login. Throwaway
// verification for the disconnect fix; requires a signed-in agy.
func TestLiveAntigravityDisconnectProbe(t *testing.T) {
	if _, err := exec.LookPath("agy"); err != nil {
		t.Skip("agy not installed")
	}
	secret, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", antigravityKeyringService, "-a", antigravityKeyringAccount, "-w").Output()
	if err != nil {
		t.Skip("no antigravity keychain entry to test against")
	}
	tokenPath := expandAuthPath("~/.gemini/antigravity-cli/antigravity-oauth-token")
	token, tokenErr := os.ReadFile(tokenPath)

	restore := func() {
		_ = exec.Command("/usr/bin/security", "delete-generic-password", "-s", antigravityKeyringService, "-a", antigravityKeyringAccount).Run()
		if err := exec.Command("/usr/bin/security", "add-generic-password", "-s", antigravityKeyringService, "-a", antigravityKeyringAccount, "-w", strings.TrimSpace(string(secret))).Run(); err != nil {
			t.Errorf("restore keychain: %v", err)
		}
		if tokenErr == nil {
			if err := os.WriteFile(tokenPath, token, 0o600); err != nil {
				t.Errorf("restore token file: %v", err)
			}
		}
	}
	t.Cleanup(restore)

	agyModels := func() error {
		cmd := exec.Command("agy", "models")
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			return err
		case <-time.After(20 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatal("agy models hung")
			return nil
		}
	}

	if err := agyModels(); err != nil {
		t.Skipf("agy not signed in before probe: %v", err)
	}
	if err := RemoveOwnedCredential(AgentAntigravity, tokenPath, t.TempDir()); err != nil {
		t.Fatalf("RemoveOwnedCredential: %v", err)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("token file still present: %v", err)
	}
	if err := exec.Command("/usr/bin/security", "find-generic-password", "-s", antigravityKeyringService, "-a", antigravityKeyringAccount).Run(); err == nil {
		t.Fatal("keychain entry still present after removal")
	}
	if err := agyModels(); err == nil {
		t.Fatal("agy still signed in after credential removal")
	}
	restore()
	if err := agyModels(); err != nil {
		t.Fatalf("agy not signed in after restore: %v", err)
	}
}
