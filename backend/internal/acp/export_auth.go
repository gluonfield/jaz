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

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

var (
	ErrUnsupportedCloudSyncAgent = errors.New("unsupported agent for cloud profile sync")
	ErrLocalAuthNotFound         = errors.New("no local login credentials found for agent")
)

type LocalAuthExport struct {
	Agent       string
	Files       map[string][]byte
	StoragePath string
	OwnedByJaz  bool
}

func (m *Manager) ExportLocalAuth(agent string) (LocalAuthExport, error) {
	name := CanonicalAgentName(agent)
	var cfg AgentConfig
	if m.agents != nil {
		if found, ok, err := m.agents.AgentConfig(name); err == nil && ok {
			cfg = found
		}
	}
	return ExportLocalAuthFiles(name, cfg, m.cfg.Root, nil, m.providers())
}

func (m *Manager) ClearOwnedLocalAuth(agent, storagePath string) error {
	return RemoveOwnedCredential(agent, storagePath, m.cfg.Root)
}

func ExportLocalAuthFiles(agent string, cfg AgentConfig, root string, env map[string]string, providers map[string]modelprovider.ModelProviderConfig) (LocalAuthExport, error) {
	name := CanonicalAgentName(agent)
	switch name {
	case AgentCodex, AgentClaude, AgentAntigravity, AgentKimi:
	default:
		return LocalAuthExport{}, fmt.Errorf("%w: %s", ErrUnsupportedCloudSyncAgent, agent)
	}

	resolved := resolveAgentAuthWithProviders(name, cfg, root, env, providers)
	files := make(map[string][]byte)
	var storagePath string

	switch name {
	case AgentCodex:
		dir := strings.TrimSpace(resolved.Config.Path)
		if dir == "" {
			dir = expandAuthPath(firstNonEmpty(os.Getenv("CODEX_HOME"), defaultHomePath(".codex")))
		}
		if data, ok := readRegularNonEmptyFile(filepath.Join(dir, "auth.json")); ok {
			files["auth.json"] = data
			storagePath = filepath.Join(dir, "auth.json")
		}
		if data, ok := readRegularNonEmptyFile(filepath.Join(dir, "config.toml")); ok {
			files["config.toml"] = data
		}
		if len(files["auth.json"]) == 0 {
			return LocalAuthExport{}, fmt.Errorf("%w: sign in to Codex locally first (expected auth.json in %s)", ErrLocalAuthNotFound, dir)
		}

	case AgentClaude:
		dir := strings.TrimSpace(resolved.Config.Path)
		if dir == "" {
			dir = expandAuthPath(firstNonEmpty(os.Getenv("CLAUDE_CONFIG_DIR"), defaultHomePath(".claude")))
		}
		if data, ok := readClaudeCredentialsBlob(dir); ok {
			files[".credentials.json"] = data
			storagePath = filepath.Join(dir, ".credentials.json")
		}
		if data, ok := readRegularNonEmptyFile(filepath.Join(dir, ".claude.json")); ok {
			files[".claude.json"] = data
		} else if dir == expandAuthPath(defaultHomePath(".claude")) {
			if data, ok := readRegularNonEmptyFile(expandAuthPath(defaultHomePath(".claude.json"))); ok {
				files[".claude.json"] = data
			}
		}
		if len(files[".credentials.json"]) == 0 {
			return LocalAuthExport{}, fmt.Errorf("%w: sign in to Claude Code locally first", ErrLocalAuthNotFound)
		}

	case AgentKimi:
		dir := strings.TrimSpace(resolved.Config.Path)
		if dir == "" {
			dir = expandAuthPath(firstNonEmpty(os.Getenv("KIMI_CODE_HOME"), defaultHomePath(".kimi-code")))
		}
		if data, ok := readRegularNonEmptyFile(filepath.Join(dir, "config.toml")); ok {
			files["config.toml"] = data
		}
		credDir := filepath.Join(dir, "credentials")
		if entries, err := os.ReadDir(credDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				full := filepath.Join(credDir, entry.Name())
				if data, ok := readRegularNonEmptyFile(full); ok {
					files["credentials/"+entry.Name()] = data
					if storagePath == "" {
						storagePath = full
					}
				}
			}
		}
		if storagePath == "" {
			return LocalAuthExport{}, fmt.Errorf("%w: sign in to Kimi locally first", ErrLocalAuthNotFound)
		}

	case AgentAntigravity:
		searchDirs := []string{}
		if dir := strings.TrimSpace(resolved.Config.Path); dir != "" {
			searchDirs = append(searchDirs, dir)
		}
		if geminiDir := expandAuthPath(defaultHomePath(".gemini")); geminiDir != "" {
			searchDirs = append(searchDirs, geminiDir)
		}
		for _, dir := range searchDirs {
			for _, rel := range []string{"oauth_creds.json", "accounts.json", "google_accounts.json"} {
				if _, exists := files[rel]; exists {
					continue
				}
				full := filepath.Join(dir, rel)
				if data, ok := readRegularNonEmptyFile(full); ok {
					files[rel] = data
					if storagePath == "" {
						storagePath = full
					}
				}
			}
		}
		if len(files) == 0 {
			return LocalAuthExport{}, fmt.Errorf("%w: Antigravity stores OAuth in the OS keyring and has no portable credential files to sync", ErrLocalAuthNotFound)
		}
	}

	if storagePath == "" {
		storagePath = resolved.StoragePath
	}
	return LocalAuthExport{
		Agent:       name,
		Files:       files,
		StoragePath: storagePath,
		OwnedByJaz:  storagePath != "" && pathUnderRoot(storagePath, root),
	}, nil
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

func readClaudeCredentialsBlob(configDir string) ([]byte, bool) {
	if data, ok := readRegularNonEmptyFile(filepath.Join(configDir, ".credentials.json")); ok {
		if tok, valid := claudeAccessToken(data); valid && tok != "" {
			return data, true
		}
	}
	if runtime.GOOS != "darwin" {
		return nil, false
	}
	services := []string{claudeKeychainService(configDir)}
	if configDir == expandAuthPath(defaultHomePath(".claude")) {
		services = append(services, "Claude Code-credentials")
	}
	for _, svc := range services {
		ctx, cancel := context.WithTimeout(context.Background(), claudeCredentialReadLimit)
		out, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", svc, "-w").Output()
		cancel()
		if err != nil {
			continue
		}
		if tok, valid := claudeAccessToken(out); valid && tok != "" {
			return []byte(strings.TrimSpace(string(out))), true
		}
	}
	return nil, false
}
