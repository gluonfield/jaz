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
)

const (
	macOSSecurityItemNotFound = 44
	claudeCredentialReadLimit = 5 * time.Second
)

func claudeSDKConfigDir(configDir string, env map[string]string) string {
	if strings.TrimSpace(env["ANTHROPIC_PROFILE"]) != "" {
		return env["ANTHROPIC_CONFIG_DIR"]
	}
	return firstNonEmpty(env["ANTHROPIC_CONFIG_DIR"], filepath.Join(configDir, "anthropic"))
}

func (j *jobState) claudeAuthFailed() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.ACPAgent == AgentClaude && j.State == StateFailed && j.turn == nil && j.finishing == nil &&
		claudeAuthFailure(j.Error)
}

func claudeAuthFailure(message string) bool {
	if strings.HasPrefix(message, signInHint+":") {
		return true
	}
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

// claudeProfileAuthAvailable reports whether a Claude profile Jaz can name still
// holds a usable login. When Anthropic retires a refresh token Claude Code blanks
// .credentials.json in place while leaving .claude.json's oauthAccount behind.
// On macOS, passive probes only check Keychain item metadata (without -w) so Jaz
// never triggers an ACL password prompt for Anthropic's Keychain entry.
func claudeProfileAuthAvailable(configDir string) bool {
	if token, ok := claudeStoredAccessToken(configDir); ok {
		return token != ""
	}
	if exists, known := claudeKeychainItemExists(configDir); known {
		return exists
	}
	return claudeAuthFileAvailable(configDir)
}

func claudeStoredAccessToken(configDir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(configDir, ".credentials.json"))
	if err != nil {
		return "", false
	}
	return claudeAccessToken(data)
}

var (
	claudeKeychainGOOS = func() string { return runtime.GOOS }
	claudeSecurityRun  = func(ctx context.Context, args ...string) error {
		return exec.CommandContext(ctx, "/usr/bin/security", args...).Run()
	}
)

type exitCoder interface {
	ExitCode() int
}

func claudeKeychainItemExists(configDir string) (exists bool, known bool) {
	if claudeKeychainGOOS() != "darwin" {
		return false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), claudeCredentialReadLimit)
	defer cancel()
	err := claudeSecurityRun(ctx, "find-generic-password", "-s", claudeKeychainService(configDir))
	if err == nil {
		return true, true
	}
	if exitErr, ok := err.(exitCoder); ok && exitErr.ExitCode() == macOSSecurityItemNotFound {
		return false, true
	}
	return false, false
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
	for _, name := range []string{".claude.json", ".credentials.json", ".jaz-auth-failed"} {
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
