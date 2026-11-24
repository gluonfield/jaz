package acp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrUnsupportedCloudLoginAgent = errors.New("isolated desktop OAuth login is not supported for this agent")
	ErrIsolatedAuthNotCaptured    = errors.New("sign-in finished without saving credentials")
)

type IsolatedCloudLogin struct {
	Agent      string
	Dir        string
	Invocation AgentLoginInvocation
	Cleanup    func()
}

// StartIsolatedCloudLogin prepares a fresh 0700 profile directory so Desktop can
// run an independent browser OAuth sign-in for a Cloud workspace without touching
// or sharing the user's Local workspace credentials.
func (m *Manager) StartIsolatedCloudLogin(agent string) (IsolatedCloudLogin, error) {
	name := CanonicalAgentName(agent)
	var cfg AgentConfig
	if m.agents != nil {
		if found, ok, err := m.agents.AgentConfig(name); err == nil && ok {
			cfg = found
		}
	}
	return PrepareIsolatedCloudLogin(name, cfg, m.cfg.Root)
}

func PrepareIsolatedCloudLogin(agent string, cfg AgentConfig, root string) (IsolatedCloudLogin, error) {
	name := CanonicalAgentName(agent)
	switch name {
	case AgentClaude, AgentKimi:
	default:
		return IsolatedCloudLogin{}, fmt.Errorf("%w: %s", ErrUnsupportedCloudLoginAgent, agent)
	}
	dir, err := os.MkdirTemp("", "jaz-cloud-acp-*")
	if err != nil {
		return IsolatedCloudLogin{}, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return IsolatedCloudLogin{}, err
	}
	auth := AgentAuthConfig{Mode: AuthModeJazProfile, Path: dir}
	inv := AgentLoginInvocationFor(name, root, auth, loginBinDirs(cfg), cfg.Env)
	if !inv.Available {
		_ = os.RemoveAll(dir)
		reason := strings.TrimSpace(inv.Reason)
		if reason == "" {
			reason = fmt.Sprintf("%s CLI is not installed locally", name)
		}
		return IsolatedCloudLogin{}, errors.New(reason)
	}
	cleanup := func() {
		if name == AgentClaude {
			_ = removeClaudeProfileKeychainCredential(dir)
		}
		_ = os.RemoveAll(dir)
	}
	return IsolatedCloudLogin{
		Agent:      name,
		Dir:        dir,
		Invocation: inv,
		Cleanup:    cleanup,
	}, nil
}

// CaptureIsolatedAuthFiles reads credential files written inside an isolated
// login directory (including macOS Keychain fallback for Claude Code).
func CaptureIsolatedAuthFiles(agent, dir string) (map[string][]byte, error) {
	name := CanonicalAgentName(agent)
	files := make(map[string][]byte)
	switch name {
	case AgentClaude:
		if data, ok := readIsolatedClaudeCredentials(dir); ok {
			files[".credentials.json"] = data
		}
		if data, ok := readRegularNonEmptyFile(filepath.Join(dir, ".claude.json")); ok {
			files[".claude.json"] = data
		}
		if len(files[".credentials.json"]) == 0 {
			return nil, fmt.Errorf("%w for claude", ErrIsolatedAuthNotCaptured)
		}
	case AgentKimi:
		if data, ok := readRegularNonEmptyFile(filepath.Join(dir, "config.toml")); ok {
			files["config.toml"] = data
		}
		credDir := filepath.Join(dir, "credentials")
		if entries, err := os.ReadDir(credDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				if data, ok := readRegularNonEmptyFile(filepath.Join(credDir, entry.Name())); ok {
					files["credentials/"+entry.Name()] = data
				}
			}
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("%w for kimi", ErrIsolatedAuthNotCaptured)
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedCloudLoginAgent, agent)
	}
	return files, nil
}

func readRegularNonEmptyFile(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func readIsolatedClaudeCredentials(configDir string) ([]byte, bool) {
	if data, ok := readRegularNonEmptyFile(filepath.Join(configDir, ".credentials.json")); ok {
		if tok, valid := claudeAccessToken(data); valid && tok != "" {
			return data, true
		}
	}
	if runtime.GOOS != "darwin" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), claudeCredentialReadLimit)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", claudeKeychainService(configDir), "-w").Output()
	if err != nil {
		return nil, false
	}
	if tok, valid := claudeAccessToken(out); valid && tok != "" {
		return []byte(strings.TrimSpace(string(out))), true
	}
	return nil, false
}
