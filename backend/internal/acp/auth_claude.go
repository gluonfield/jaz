package acp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/runtimefiles"
)

const (
	authFailureMarker         = ".jaz-auth-failed"
	macOSSecurityItemNotFound = 44
	claudeCredentialReadLimit = 5 * time.Second
)

func (m *Manager) recordRuntimeAuthFailure(job *jobState, message string) {
	if CanonicalAgentName(job.ACPAgent) != AgentClaude {
		return
	}
	cfg, ok, err := m.configuredAgent(job.ACPAgent)
	if err != nil || !ok {
		return
	}
	if err := recordClaudeAuthFailure(cfg.Auth, m.cfg.Root, message); err != nil {
		m.log.Warn("record acp auth failure", "session", job.ID, "error", err)
	}
}

func recordClaudeAuthFailure(auth AgentAuthConfig, root, message string) error {
	if auth.Mode == AuthModeExistingCLI || !claudeAuthFailure(message) {
		return nil
	}
	configDir := runtimefiles.New(root).ACPClaudeConfig
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, authFailureMarker), []byte(strings.TrimSpace(message)+"\n"), 0o600)
}

func claudeAuthFailure(message string) bool {
	message = strings.ToLower(message)
	for _, text := range []string{
		"authentication required",
		"invalid authentication credentials",
		"login expired",
		"oauth access token has been revoked",
		"please run /login",
	} {
		if strings.Contains(message, text) {
			return true
		}
	}
	return false
}

func claudeAuthFailureRecorded(configDir string) bool {
	return fileExists(filepath.Join(configDir, authFailureMarker))
}

// claudeProfileAuthAvailable reports whether a Claude profile Jaz can name still
// holds a usable login. When Anthropic retires a refresh token Claude Code blanks
// the stored tokens in place and leaves .claude.json's oauthAccount behind, so
// profile metadata on its own keeps reporting a connected agent over a credential
// that can no longer authenticate. A credential store Jaz cannot read stays
// unknown and falls back to that metadata rather than guessing a logout.
func claudeProfileAuthAvailable(configDir string) bool {
	if token, ok := claudeStoredAccessToken(configDir); ok {
		return token != ""
	}
	return claudeAuthFileAvailable(configDir)
}

func claudeStoredAccessToken(configDir string) (string, bool) {
	if data, err := os.ReadFile(filepath.Join(configDir, ".credentials.json")); err == nil {
		return claudeAccessToken(data)
	}
	if runtime.GOOS != "darwin" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), claudeCredentialReadLimit)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", claudeKeychainService(configDir), "-w").Output()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == macOSSecurityItemNotFound {
		return "", true
	}
	if err != nil {
		return "", false
	}
	return claudeAccessToken(out)
}

func claudeAccessToken(data []byte) (string, bool) {
	var stored struct {
		OAuth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return "", false
	}
	return strings.TrimSpace(stored.OAuth.AccessToken), true
}

func removeClaudeProfileCredentials(configDir string) error {
	if err := removeClaudeProfileKeychainCredential(configDir); err != nil {
		return err
	}
	for _, name := range []string{".claude.json", ".credentials.json", authFailureMarker} {
		if err := os.Remove(filepath.Join(configDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func removeClaudeProfileKeychainCredential(configDir string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	output, err := exec.Command("/usr/bin/security", "delete-generic-password", "-s", claudeKeychainService(configDir)).CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == macOSSecurityItemNotFound {
		return nil
	}
	if err == nil {
		return nil
	}
	return fmt.Errorf("delete Claude profile credential from Keychain: %s: %w", strings.TrimSpace(string(output)), err)
}

func claudeKeychainService(configDir string) string {
	// Claude Code scopes custom profiles by the first eight hex digits of the config-path SHA-256.
	sum := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("Claude Code-credentials-%x", sum[:4])
}
