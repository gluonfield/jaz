package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/gluonfield/acp-transport/streamhttp"
)

func (m *Manager) openConn(ctx context.Context, name string, cfg AgentConfig, env map[string]string, cwd string) (jsonrpc.MessageConn, error) {
	if cfg.URL != "" {
		opts := []streamhttp.ClientOption{}
		parsed, err := url.Parse(cfg.URL)
		if err != nil {
			return nil, err
		}
		if parsed.Scheme == "http" {
			opts = append(opts, streamhttp.WithH2C())
		}
		if cfg.Token != "" {
			opts = append(opts, streamhttp.WithBearerToken(cfg.Token))
		}
		return streamhttp.Dial(cfg.URL, opts...)
	}
	if cfg.Command == "" {
		return nil, fmt.Errorf("acp agent %q has no command", name)
	}
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Env = envList(env)
	if cwd != "" {
		cmd.Dir = cwd
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start acp agent %q (%s): %w", name, strings.Join(append([]string{cfg.Command}, cfg.Args...), " "), err)
	}
	conn := stdio.New(stdout, stdin)
	go func() {
		_ = cmd.Wait()
		_ = conn.Close()
	}()
	return conn, nil
}

func (m *Manager) processEnv(name string, agent AgentConfig) map[string]string {
	env := map[string]string{}
	for _, key := range []string{"PATH", "CODEX_HOME"} {
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}
	for key, value := range m.cfg.Env {
		env[key] = value
	}
	for key, value := range agent.Env {
		env[key] = value
	}
	normalizeEnv(env, "OPENAI_API_KEY", "OPENAI_APIKEY")
	normalizeEnv(env, "ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY")

	root := firstNonEmpty(m.cfg.Root, filepath.Join(os.TempDir(), "jaz"))
	home := filepath.Join(root, "acp", "home")
	if name == AgentCodex {
		if codexHome := prepareCodexHome(root, env["CODEX_HOME"]); codexHome != "" {
			env["CODEX_HOME"] = codexHome
		}
		for _, key := range []string{"OPENAI_API_KEY", "OPENAI_APIKEY", "OPENROUTER_API_KEY", "OPENROUTER_APIKEY", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN"} {
			delete(env, key)
		}
	}
	if name == AgentClaudeCode {
		configuredHome := strings.TrimSpace(env["HOME"])
		preserveHostEnv(env, []string{
			"ANTHROPIC_API_KEY",
			"ANTHROPIC_APIKEY",
			"ANTHROPIC_AUTH_TOKEN",
			"ANTHROPIC_BASE_URL",
			"CLAUDE_CODE_EXECUTABLE",
			"CLAUDE_CODE_OAUTH_TOKEN",
			"CLAUDE_CODE_USE_BEDROCK",
			"CLAUDE_CODE_USE_VERTEX",
			"CLAUDE_CONFIG_DIR",
			"LANG",
			"LC_ALL",
			"LC_CTYPE",
			"LOGNAME",
			"SHELL",
			"SSH_AUTH_SOCK",
			"USER",
		})
		if configuredHome != "" {
			home = configuredHome
		} else if userHome, err := os.UserHomeDir(); err == nil && userHome != "" {
			home = userHome
		}
		if env["CLAUDE_CODE_EXECUTABLE"] == "" {
			if cli, err := exec.LookPath("claude"); err == nil {
				env["CLAUDE_CODE_EXECUTABLE"] = cli
			}
		}
		normalizeEnv(env, "ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY")
	}
	tmp := filepath.Join(root, "acp", "tmp")
	cache := filepath.Join(root, "acp", "npm-cache")
	_ = os.MkdirAll(home, 0o700)
	_ = os.MkdirAll(tmp, 0o700)
	_ = os.MkdirAll(cache, 0o700)
	env["HOME"] = home
	env["TMPDIR"] = tmp
	env["TMP"] = tmp
	env["TEMP"] = tmp
	env["npm_config_cache"] = cache
	env["npm_config_ignore_scripts"] = "true"
	env["npm_config_audit"] = "false"
	env["npm_config_fund"] = "false"
	env["npm_config_update_notifier"] = "false"
	return env
}

func prepareCodexHome(root, sourceHome string) string {
	if sourceHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		sourceHome = filepath.Join(home, ".codex")
	}
	src := filepath.Join(sourceHome, "auth.json")
	if !fileExists(src) {
		return ""
	}
	dstHome := filepath.Join(root, "acp", "codex-home")
	dst := filepath.Join(dstHome, "auth.json")
	_ = os.MkdirAll(dstHome, 0o700)
	if !fileExists(dst) {
		if err := os.Symlink(src, dst); err != nil {
			if data, err := os.ReadFile(src); err == nil {
				_ = os.WriteFile(dst, data, 0o600)
			}
		}
	}
	if fileExists(dst) {
		return dstHome
	}
	return ""
}

func preserveHostEnv(env map[string]string, keys []string) {
	for _, key := range keys {
		if env[key] != "" {
			continue
		}
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}
}

func normalizeEnv(env map[string]string, canonical, alias string) {
	if env[canonical] == "" {
		env[canonical] = env[alias]
	}
	delete(env, alias)
}

func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key, value := range env {
		if value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func autoAuthMethod(agent string, raw json.RawMessage, env map[string]string) (string, []string) {
	var init struct {
		AuthMethods []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Vars []struct {
				Name string `json:"name"`
			} `json:"vars"`
		} `json:"authMethods"`
	}
	if err := json.Unmarshal(raw, &init); err != nil {
		return "", nil
	}
	if agent == AgentCodex {
		for _, method := range init.AuthMethods {
			if method.ID == "chatgpt" && codexAuthAvailable(env) {
				return method.ID, nil
			}
		}
	}
	var missing []string
	if agent == AgentCodex {
		for _, method := range init.AuthMethods {
			if method.ID == "chatgpt" {
				missing = appendMissing(missing, codexAuthHint(env))
				break
			}
		}
	}
	for _, method := range init.AuthMethods {
		if method.Type != "env_var" && len(method.Vars) == 0 {
			continue
		}
		if agent == AgentCodex {
			continue
		}
		allSet := len(method.Vars) > 0
		for _, v := range method.Vars {
			if env[v.Name] == "" {
				allSet = false
				missing = appendMissing(missing, v.Name)
				break
			}
		}
		if allSet {
			return method.ID, nil
		}
	}
	return "", missing
}

func codexAuthAvailable(env map[string]string) bool {
	home := env["CODEX_HOME"]
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		home = filepath.Join(userHome, ".codex")
	}
	return fileExists(filepath.Join(home, "auth.json"))
}

func codexAuthHint(env map[string]string) string {
	if env["CODEX_HOME"] != "" {
		return "Codex OAuth login at " + filepath.Join(env["CODEX_HOME"], "auth.json")
	}
	return "Codex OAuth login at ~/.codex/auth.json"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func appendMissing(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
