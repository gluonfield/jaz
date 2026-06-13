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
	"github.com/wins/jaz/backend/internal/runtimefiles"
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
	command, args := processCommand(name, cfg)
	if resolved, err := ResolveExecutable(command); err == nil {
		command = resolved
	}
	cmd := exec.CommandContext(ctx, command, args...)
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
		return nil, fmt.Errorf("start acp agent %q (%s): %w", name, strings.Join(append([]string{command}, args...), " "), err)
	}
	conn := stdio.New(stdout, stdin)
	go func() {
		_ = cmd.Wait()
		_ = conn.Close()
	}()
	return conn, nil
}

func (m *Manager) processEnv(name string, agent AgentConfig) map[string]string {
	env, _ := m.buildProcessEnv(name, agent, true)
	return env
}

func (m *Manager) processEnvPrepared(name string, agent AgentConfig) (map[string]string, error) {
	return m.buildProcessEnv(name, agent, true)
}

func (m *Manager) probeEnv(name string, agent AgentConfig) map[string]string {
	env, _ := m.buildProcessEnv(name, agent, false)
	return env
}

func (m *Manager) buildProcessEnv(name string, agent AgentConfig, prepare bool) (map[string]string, error) {
	name = CanonicalAgentName(name)
	env := map[string]string{}
	var prepareErr error
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
	normalizeEnv(env, "XAI_API_KEY", "XAI_APIKEY")

	root := firstNonEmpty(m.cfg.Root, filepath.Join(os.TempDir(), "jaz"))
	layout := runtimefiles.New(root)
	home := layout.ACPHome
	if name == AgentCodex {
		codexHome := layout.ACPCodexHome
		if prepare {
			var err error
			codexHome, err = prepareCodexHome(root, env["CODEX_HOME"])
			prepareErr = firstError(prepareErr, err)
		}
		if codexHome != "" {
			env["CODEX_HOME"] = codexHome
		}
		for _, key := range []string{"OPENAI_API_KEY", "OPENAI_APIKEY", "OPENROUTER_API_KEY", "OPENROUTER_APIKEY", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN"} {
			delete(env, key)
		}
	}
	if name == AgentClaude {
		configuredHome := strings.TrimSpace(env["HOME"])
		configuredConfigDir := strings.TrimSpace(env["CLAUDE_CONFIG_DIR"])
		preserveHostEnv(env, []string{
			"ANTHROPIC_API_KEY",
			"ANTHROPIC_APIKEY",
			"ANTHROPIC_AUTH_TOKEN",
			"ANTHROPIC_BASE_URL",
			"CLAUDE_CODE_EXECUTABLE",
			"CLAUDE_CODE_OAUTH_TOKEN",
			"CLAUDE_CODE_USE_BEDROCK",
			"CLAUDE_CODE_USE_VERTEX",
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
		}
		if configuredConfigDir != "" {
			env["CLAUDE_CONFIG_DIR"] = configuredConfigDir
		} else {
			env["CLAUDE_CONFIG_DIR"] = layout.ACPClaudeConfig
			if prepare {
				var err error
				env["CLAUDE_CONFIG_DIR"], err = prepareClaudeConfig(root, os.Getenv("CLAUDE_CONFIG_DIR"))
				prepareErr = firstError(prepareErr, err)
			}
		}
		if env["CLAUDE_CODE_EXECUTABLE"] == "" {
			if cli, err := ResolveExecutable("claude"); err == nil {
				env["CLAUDE_CODE_EXECUTABLE"] = cli
			}
		}
		normalizeEnv(env, "ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY")
	}
	if name == AgentGrok {
		configuredHome := strings.TrimSpace(env["HOME"])
		preserveHostEnv(env, []string{
			"HTTP_PROXY",
			"HTTPS_PROXY",
			"LANG",
			"LC_ALL",
			"LC_CTYPE",
			"LOGNAME",
			"NO_PROXY",
			"SHELL",
			"SSH_AUTH_SOCK",
			"USER",
			"XAI_API_KEY",
			"XAI_APIKEY",
		})
		if configuredHome != "" {
			home = configuredHome
		} else {
			home = layout.ACPHome
			if prepare {
				var err error
				home, err = prepareGrokHome(root, "")
				prepareErr = firstError(prepareErr, err)
			}
		}
		normalizeEnv(env, "XAI_API_KEY", "XAI_APIKEY")
	}
	tmp := layout.ACPTmp
	cache := layout.ACPNPMCache
	if prepare {
		for _, dir := range []string{home, tmp, cache} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				prepareErr = firstError(prepareErr, fmt.Errorf("prepare acp directory %s: %w", dir, err))
			}
		}
	}
	env["HOME"] = home
	env["TMPDIR"] = tmp
	env["TMP"] = tmp
	env["TEMP"] = tmp
	env["npm_config_cache"] = cache
	env["npm_config_ignore_scripts"] = "true"
	env["npm_config_audit"] = "false"
	env["npm_config_fund"] = "false"
	env["npm_config_update_notifier"] = "false"
	return env, prepareErr
}

func processCommand(name string, cfg AgentConfig) (string, []string) {
	args := append([]string(nil), cfg.Args...)
	if CanonicalAgentName(name) == AgentGrok && isGrokCommand(cfg.Command) {
		args = withGrokAlwaysApproveArg(args)
		args = withGrokReasoningEffortArg(args, configuredReasoningEffort(cfg.ReasoningEffort))
	}
	return cfg.Command, args
}

func isGrokCommand(command string) bool {
	return filepath.Base(strings.TrimSpace(command)) == "grok"
}

func withGrokReasoningEffortArg(args []string, effort string) []string {
	if strings.TrimSpace(effort) == "" || hasFlag(args, "--reasoning-effort", "--effort") {
		return args
	}
	insertAt := len(args)
	for i, arg := range args {
		if arg == "stdio" {
			insertAt = i
			break
		}
	}
	next := make([]string, 0, len(args)+2)
	next = append(next, args[:insertAt]...)
	next = append(next, "--reasoning-effort", effort)
	next = append(next, args[insertAt:]...)
	return next
}

func withGrokAlwaysApproveArg(args []string) []string {
	if hasFlag(args, "--always-approve") || hasFlag(args, "--permission-mode") {
		return args
	}
	return insertBeforeArg(args, "stdio", "--always-approve")
}

func insertBeforeArg(args []string, marker string, values ...string) []string {
	insertAt := len(args)
	for i, arg := range args {
		if arg == marker {
			insertAt = i
			break
		}
	}
	next := make([]string, 0, len(args)+len(values))
	next = append(next, args[:insertAt]...)
	next = append(next, values...)
	next = append(next, args[insertAt:]...)
	return next
}

func hasFlag(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name || strings.HasPrefix(arg, name+"=") {
				return true
			}
		}
	}
	return false
}

func prepareCodexHome(root, sourceHome string) (string, error) {
	dstHome := runtimefiles.New(root).ACPCodexHome
	if sourceHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return dstHome, nil
		}
		sourceHome = filepath.Join(home, ".codex")
	}
	src := filepath.Join(sourceHome, "auth.json")
	dst := filepath.Join(dstHome, "auth.json")
	if err := os.MkdirAll(dstHome, 0o700); err != nil {
		return dstHome, fmt.Errorf("prepare codex auth: %w", err)
	}
	if fileExists(src) && !fileExists(dst) {
		if err := copyFile(src, dst); err != nil {
			return dstHome, fmt.Errorf("prepare codex auth: %w", err)
		}
	}
	return dstHome, nil
}

func prepareGrokHome(root, sourceHome string) (string, error) {
	dstHome := runtimefiles.New(root).ACPHome
	if sourceHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return dstHome, nil
		}
		sourceHome = home
	}
	src := filepath.Join(sourceHome, ".grok", "auth.json")
	dstDir := filepath.Join(dstHome, ".grok")
	dst := filepath.Join(dstDir, "auth.json")
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		return dstHome, fmt.Errorf("prepare grok auth: %w", err)
	}
	if fileExists(src) && !fileExists(dst) {
		if err := copyFile(src, dst); err != nil {
			return dstHome, fmt.Errorf("prepare grok auth: %w", err)
		}
	}
	return dstHome, nil
}

func prepareClaudeConfig(root, sourceDir string) (string, error) {
	dstDir := runtimefiles.New(root).ACPClaudeConfig
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		return dstDir, fmt.Errorf("prepare claude auth: %w", err)
	}
	dst := filepath.Join(dstDir, ".credentials.json")
	for _, src := range claudeCredentialCandidates(sourceDir) {
		if fileExists(src) && !fileExists(dst) {
			if err := copyFile(src, dst); err != nil {
				return dstDir, fmt.Errorf("prepare claude auth: %w", err)
			}
			break
		}
	}
	return dstDir, nil
}

func claudeCredentialCandidates(sourceDir string) []string {
	candidates := []string{}
	if strings.TrimSpace(sourceDir) != "" {
		candidates = append(candidates, filepath.Join(sourceDir, ".credentials.json"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".claude", ".credentials.json"))
	}
	return candidates
}

func copyFile(src, dst string) error {
	if src == dst {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return nil
}

func firstError(current, next error) error {
	if current != nil {
		return current
	}
	return next
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
	if agent == AgentGrok {
		for _, method := range init.AuthMethods {
			if method.ID == "xai.api_key" && env["XAI_API_KEY"] != "" {
				return method.ID, nil
			}
		}
		for _, method := range init.AuthMethods {
			if method.ID == "cached_token" && grokAuthAvailable(env) {
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
	if agent == AgentGrok {
		for _, method := range init.AuthMethods {
			if method.ID == "cached_token" || method.ID == "xai.api_key" || method.ID == "grok.com" {
				missing = appendMissing(missing, grokAuthHint(env))
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

func claudeAuthAvailable(env map[string]string) bool {
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		if strings.TrimSpace(env[key]) != "" {
			return true
		}
	}
	configDir := env["CLAUDE_CONFIG_DIR"]
	if configDir == "" {
		return false
	}
	return fileExists(filepath.Join(configDir, ".credentials.json"))
}

func claudeAuthHint(env map[string]string) string {
	if env["CLAUDE_CONFIG_DIR"] != "" {
		return "Claude login at " + filepath.Join(env["CLAUDE_CONFIG_DIR"], ".credentials.json")
	}
	return "Claude login at ~/.claude/.credentials.json"
}

func grokAuthAvailable(env map[string]string) bool {
	home := env["HOME"]
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		home = userHome
	}
	return fileExists(filepath.Join(home, ".grok", "auth.json"))
}

func grokAuthHint(env map[string]string) string {
	if env["HOME"] != "" {
		return "Grok login at " + filepath.Join(env["HOME"], ".grok", "auth.json") + " or XAI_API_KEY"
	}
	return "Grok login at ~/.grok/auth.json or XAI_API_KEY"
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
