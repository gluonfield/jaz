package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/gluonfield/acp-transport/streamhttp"
	"github.com/wins/jaz/backend/internal/skills"
)

const processStderrTailLimit = 2000

type processStderrTail struct {
	mu   sync.Mutex
	text string
	done chan struct{}
}

func newProcessStderrTail() *processStderrTail {
	return &processStderrTail{done: make(chan struct{})}
}

func (t *processStderrTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.text += string(p)
	if len(t.text) > processStderrTailLimit {
		t.text = t.text[len(t.text)-processStderrTailLimit:]
	}
	return len(p), nil
}

func (t *processStderrTail) close() {
	close(t.done)
}

func (t *processStderrTail) String() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(t.text)
}

func withProcessStderr(err error, stderr *processStderrTail) error {
	if stderr != nil {
		select {
		case <-stderr.done:
		case <-time.After(100 * time.Millisecond):
		}
	}
	if detail := stderr.String(); detail != "" {
		return fmt.Errorf("%w: %s", err, detail)
	}
	return err
}

func (m *Manager) openConn(ctx context.Context, name string, cfg AgentConfig, env map[string]string, cwd string) (jsonrpc.MessageConn, *processStderrTail, error) {
	if cfg.URL != "" {
		opts := []streamhttp.ClientOption{}
		parsed, err := url.Parse(cfg.URL)
		if err != nil {
			return nil, nil, err
		}
		if parsed.Scheme == "http" {
			opts = append(opts, streamhttp.WithH2C())
		}
		if cfg.Token != "" {
			opts = append(opts, streamhttp.WithBearerToken(cfg.Token))
		}
		conn, err := streamhttp.Dial(cfg.URL, opts...)
		return conn, nil, err
	}
	if cfg.Command == "" {
		return nil, nil, fmt.Errorf("acp agent %q has no command", name)
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
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr := newProcessStderrTail()
	cmd.Stderr = io.MultiWriter(os.Stderr, stderr)
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start acp agent %q (%s): %w", name, strings.Join(append([]string{command}, args...), " "), err)
	}
	conn := stdio.New(stdout, stdin)
	go func() {
		_ = cmd.Wait()
		stderr.close()
		_ = conn.Close()
	}()
	return conn, stderr, nil
}

func (m *Manager) processEnv(name string, agent AgentConfig) map[string]string {
	env, _ := m.buildProcessEnv(name, agent, "", nil, true)
	return env
}

func (m *Manager) processEnvPrepared(name string, agent AgentConfig) (map[string]string, error) {
	return m.processEnvPreparedForSurface(name, agent, "", nil)
}

func (m *Manager) processEnvPreparedForSurface(name string, agent AgentConfig, artifactSurface string, systemPromptExtensions []string) (map[string]string, error) {
	return m.buildProcessEnv(name, agent, artifactSurface, systemPromptExtensions, true)
}

func (m *Manager) probeEnv(name string, agent AgentConfig) map[string]string {
	env, _ := m.buildProcessEnv(name, agent, "", nil, false)
	return env
}

func (m *Manager) buildProcessEnv(name string, agent AgentConfig, artifactSurface string, systemPromptExtensions []string, prepare bool) (map[string]string, error) {
	name = CanonicalAgentName(name)
	env := map[string]string{}
	var prepareErr error
	for _, key := range []string{"PATH"} {
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
	delete(env, "HOME")
	normalizeEnv(env, "OPENAI_API_KEY", "OPENAI_APIKEY")
	normalizeEnv(env, "ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY")
	normalizeEnv(env, "OPENROUTER_API_KEY", "OPENROUTER_APIKEY")
	normalizeEnv(env, "XAI_API_KEY", "XAI_APIKEY")

	root := firstNonEmpty(m.cfg.Root, filepath.Join(os.TempDir(), "jaz"))
	if name == AgentCodex {
		auth := resolveAgentAuthWithProviders(name, agent, root, env, m.providers())
		codexHome := auth.Config.Path
		if codexHome != "" {
			env["CODEX_HOME"] = codexHome
			if prepare && auth.Config.Mode == AuthModeJazProfile {
				if err := os.MkdirAll(codexHome, 0o700); err != nil {
					prepareErr = firstError(prepareErr, fmt.Errorf("prepare codex profile %s: %w", codexHome, err))
				}
			}
			if prepare {
				m.installAgentSkills(name, root, filepath.Join(codexHome, "skills"))
			}
		}
		for _, key := range []string{"OPENAI_API_KEY", "OPENAI_APIKEY", "OPENROUTER_API_KEY", "OPENROUTER_APIKEY", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN"} {
			delete(env, key)
		}
		if target, value, ok := auth.APIKeyBinding(); ok {
			env[target] = value
		}
	}
	if name == AgentClaude {
		configuredConfigDir := strings.TrimSpace(env["CLAUDE_CONFIG_DIR"])
		claudeHostEnv := []string{
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
		}
		preserveHostEnv(env, claudeHostEnv)
		auth := resolveAgentAuthWithProviders(name, agent, root, env, m.providers())
		if auth.Config.Mode == AuthModeJazProfile {
			delete(env, "ANTHROPIC_AUTH_TOKEN")
			delete(env, "CLAUDE_CODE_OAUTH_TOKEN")
		}
		if configuredConfigDir != "" && auth.Config.Mode != AuthModeJazProfile {
			env["CLAUDE_CONFIG_DIR"] = configuredConfigDir
		} else {
			env["CLAUDE_CONFIG_DIR"] = auth.Config.Path
			if prepare && auth.Config.Mode == AuthModeJazProfile {
				if err := os.MkdirAll(env["CLAUDE_CONFIG_DIR"], 0o700); err != nil {
					prepareErr = firstError(prepareErr, fmt.Errorf("prepare claude profile %s: %w", env["CLAUDE_CONFIG_DIR"], err))
				}
			}
		}
		if prepare && env["CLAUDE_CONFIG_DIR"] != "" {
			m.installAgentSkills(name, root, filepath.Join(env["CLAUDE_CONFIG_DIR"], "skills"))
		}
		if env["CLAUDE_CODE_EXECUTABLE"] == "" {
			if cli, err := ResolveExecutable("claude"); err == nil {
				env["CLAUDE_CODE_EXECUTABLE"] = cli
			}
		}
		normalizeEnv(env, "ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY")
		delete(env, "ANTHROPIC_API_KEY")
		if target, value, ok := auth.APIKeyBinding(); ok {
			env[target] = value
		}
	}
	if name == AgentGrok {
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
		})
		auth := resolveAgentAuthWithProviders(name, agent, root, env, m.providers())
		normalizeEnv(env, "XAI_API_KEY", "XAI_APIKEY")
		delete(env, "XAI_API_KEY")
		if target, value, ok := auth.APIKeyBinding(); ok {
			env[target] = value
		}
	}
	if name == AgentOpenCode {
		preserveHostEnv(env, []string{
			"HTTP_PROXY",
			"HTTPS_PROXY",
			"LANG",
			"LC_ALL",
			"LC_CTYPE",
			"NO_PROXY",
		})
		m.loadOpenCodeProviderEnv(env, root)
		auth := resolveAgentAuthWithProviders(name, agent, root, env, m.providers())
		if strings.TrimSpace(env["OPENCODE_CONFIG_DIR"]) == "" {
			env["OPENCODE_CONFIG_DIR"] = auth.Config.Path
		}
		if prepare {
			if err := os.MkdirAll(env["OPENCODE_CONFIG_DIR"], 0o700); err != nil {
				prepareErr = firstError(prepareErr, fmt.Errorf("prepare opencode profile %s: %w", env["OPENCODE_CONFIG_DIR"], err))
			}
			if err := m.prepareOpenCodeConfig(env, agent, artifactSurface, systemPromptExtensions); err != nil {
				prepareErr = firstError(prepareErr, err)
			}
		}
		if target, value, ok := auth.APIKeyBinding(); ok {
			env[target] = value
		}
	}
	if prepare {
		if spec, ok := resolveAgentAPIKeySpec(name); ok {
			delete(env, spec.SourceEnv)
		}
	}
	return env, prepareErr
}

func (m *Manager) installAgentSkills(agent, root, dst string) {
	if err := skills.InstallMissingTo(root, dst); err != nil {
		m.log.Warn("install acp skills failed", "agent", agent, "path", dst, "error", err)
	}
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
	if env[canonical] == "" {
		delete(env, canonical)
	}
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
			if method.ID == "cached_token" && grokAuthAvailable(env) {
				return method.ID, nil
			}
		}
		for _, method := range init.AuthMethods {
			if method.ID == "xai.api_key" && env["XAI_API_KEY"] != "" {
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
		allSet := len(method.Vars) > 0
		for _, v := range method.Vars {
			if env[v.Name] == "" {
				allSet = false
				missing = appendMissing(missing, authMissingEnvName(agent, v.Name))
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
	return codexAuthFileAvailable(home) || codexKeyringConfigured(home)
}

func codexAuthHint(env map[string]string) string {
	if env["CODEX_HOME"] != "" {
		return "Codex OAuth login at " + filepath.Join(env["CODEX_HOME"], "auth.json")
	}
	return "Codex OAuth login at ~/.codex/auth.json"
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
	return grokAuthFileAvailable(home)
}

func grokAuthHint(env map[string]string) string {
	apiKeyEnv := agentAPIKeySourceEnv(AgentGrok, "XAI_API_KEY")
	if env["HOME"] != "" {
		return "Grok login at " + filepath.Join(env["HOME"], ".grok", "auth.json") + " or " + apiKeyEnv
	}
	return "Grok login at ~/.grok/auth.json or " + apiKeyEnv
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

func authMissingEnvName(agent, targetEnv string) string {
	spec, ok := resolveAgentAPIKeySpec(agent)
	if ok && targetEnv == spec.TargetEnv {
		return spec.SourceEnv
	}
	return targetEnv
}

func agentAPIKeySourceEnv(agent, fallback string) string {
	spec, ok := resolveAgentAPIKeySpec(agent)
	if ok && spec.SourceEnv != "" {
		return spec.SourceEnv
	}
	return fallback
}
