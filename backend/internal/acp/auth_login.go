package acp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type codexLoginConfig struct {
	CredentialsStore string `toml:"cli_auth_credentials_store"`
}

func PrepareAgentLoginInvocation(name string, auth AgentAuthConfig, invocation AgentLoginInvocation) error {
	for key, dir := range invocation.Env {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("prepare %s profile %s: %w", key, dir, err)
		}
	}
	if log := strings.TrimSpace(invocation.TailLog); log != "" {
		if err := os.MkdirAll(filepath.Dir(log), 0o700); err != nil {
			return fmt.Errorf("prepare login log dir %s: %w", filepath.Dir(log), err)
		}
	}
	if cwd := strings.TrimSpace(invocation.Cwd); cwd != "" {
		if err := os.MkdirAll(cwd, 0o700); err != nil {
			return fmt.Errorf("prepare login workspace %s: %w", cwd, err)
		}
	}
	if CanonicalAgentName(name) == AgentCodex && auth.Mode == AuthModeJazProfile {
		if err := ensureCodexFileCredentialConfig(invocation.Env["CODEX_HOME"]); err != nil {
			return err
		}
	}
	return nil
}

func ensureCodexFileCredentialConfig(home string) error {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	path := filepath.Join(home, "config.toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(`cli_auth_credentials_store = "file"`+"\n"), 0o600)
	}
	if err != nil {
		return fmt.Errorf("read codex config %s: %w", path, err)
	}
	var cfg codexLoginConfig
	if err := toml.Unmarshal(data, &cfg); err == nil && strings.EqualFold(strings.TrimSpace(cfg.CredentialsStore), "file") {
		return nil
	}
	if err := os.WriteFile(path, setTopLevelTOMLString(data, "cli_auth_credentials_store", "file"), 0o600); err != nil {
		return fmt.Errorf("write codex config %s: %w", path, err)
	}
	return nil
}

func setTopLevelTOMLString(data []byte, key, value string) []byte {
	line := key + " = " + strconv.Quote(value)
	lines := strings.SplitAfter(string(data), "\n")
	for i, existing := range lines {
		trimmed := strings.TrimSpace(existing)
		if strings.HasPrefix(trimmed, "[") {
			break
		}
		if topLevelTOMLKey(existing) == key {
			ending := ""
			if strings.HasSuffix(existing, "\n") {
				ending = "\n"
			}
			lines[i] = line + ending
			return []byte(strings.Join(lines, ""))
		}
	}
	if len(data) == 0 {
		return []byte(line + "\n")
	}
	return []byte(line + "\n\n" + string(data))
}

func topLevelTOMLKey(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	before, _, ok := strings.Cut(trimmed, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(before)
}
