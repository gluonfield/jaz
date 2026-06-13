package acp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProcessEnvIsMinimalAndCanonical(t *testing.T) {
	t.Setenv("PATH", "/bin")
	t.Setenv("OPENAI_APIKEY", "host-openai-key")
	t.Setenv("ANTHROPIC_APIKEY", "host-anthropic-key")
	t.Setenv("SHOULD_NOT_LEAK", "secret")

	root := t.TempDir()
	manager := NewManager(nil, Config{
		Root: root,
		Env:  map[string]string{"EXPLICIT": "yes", "OPENAI_APIKEY": "openai-key"},
	}, nil)
	env := manager.processEnv("fake", AgentConfig{Env: map[string]string{"ANTHROPIC_APIKEY": "anthropic-key"}})

	if env["OPENAI_API_KEY"] != "openai-key" || env["ANTHROPIC_API_KEY"] != "anthropic-key" {
		t.Fatalf("auth env not normalized: %#v", env)
	}
	if _, ok := env["OPENAI_APIKEY"]; ok {
		t.Fatal("OPENAI_APIKEY alias leaked into subprocess env")
	}
	if _, ok := env["SHOULD_NOT_LEAK"]; ok {
		t.Fatal("unexpected host env leaked into subprocess env")
	}
	if env["EXPLICIT"] != "yes" || env["npm_config_ignore_scripts"] != "true" {
		t.Fatalf("expected explicit env and npm safety flags: %#v", env)
	}
	if !strings.HasPrefix(env["HOME"], filepath.Join(root, "acp")) {
		t.Fatalf("HOME = %q, want under %q", env["HOME"], root)
	}
}

// Each adapter reads its own _meta extension key; every form below appends to
// the agent's prompt rather than replacing it (a bare string would replace the
// preset on claude, and grok ignores systemPrompt entirely).
func TestSystemPromptMetaPerAgent(t *testing.T) {
	cases := []struct {
		agent string
		want  map[string]any
	}{
		{AgentClaude, map[string]any{"systemPrompt": map[string]any{"append": "jaz prompt"}}},
		{"claude-code", map[string]any{"systemPrompt": map[string]any{"append": "jaz prompt"}}},
		{AgentGrok, map[string]any{"rules": "jaz prompt"}},
		{"grok-build", map[string]any{"rules": "jaz prompt"}},
		{AgentCodex, map[string]any{"systemPrompt": "jaz prompt"}},
		{"unknown-agent", map[string]any{"systemPrompt": "jaz prompt"}},
	}
	for _, tc := range cases {
		if got := systemPromptMeta(tc.agent, "jaz prompt"); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("systemPromptMeta(%q) = %#v, want %#v", tc.agent, got, tc.want)
		}
	}
}

func TestResolveCwdRejectsWorkspaceEscape(t *testing.T) {
	workspace := t.TempDir()
	manager := NewManager(nil, Config{Workspace: workspace}, nil)

	if _, err := manager.resolveCwd(filepath.Join(workspace, "project")); err != nil {
		t.Fatalf("inside workspace rejected: %v", err)
	}
	if _, err := manager.resolveCwd(t.TempDir()); err == nil {
		t.Fatal("outside workspace accepted")
	}
}

func TestProcessEnvSetsCodexHomeFromSystemLogin(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("codex", AgentConfig{})

	want := filepath.Join(root, "acp", "codex-home")
	if env["CODEX_HOME"] != want {
		t.Fatalf("CODEX_HOME = %q", env["CODEX_HOME"])
	}
	if !fileExists(filepath.Join(want, "auth.json")) {
		t.Fatalf("isolated codex auth was not prepared")
	}
	info, err := os.Lstat(filepath.Join(want, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("codex auth import should copy credentials, not symlink them")
	}
	if env["HOME"] == home {
		t.Fatal("subprocess HOME should stay isolated")
	}
}

func TestProcessEnvPreparedReportsCredentialCopyFailure(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "acp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "acp", "codex-home"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("codex", AgentConfig{})
	if err == nil || !strings.Contains(err.Error(), "prepare codex auth") {
		t.Fatalf("err = %v, want codex auth preparation error", err)
	}
}

func TestProbeAgentAuthDoesNotImportCredentials(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	root := t.TempDir()
	status := ProbeAgentAuth(AgentCodex, AgentConfig{}, root, nil)
	if !status.Authenticated {
		t.Fatalf("source codex auth should be detected: %#v", status)
	}
	if _, err := os.Stat(filepath.Join(root, "acp", "codex-home", "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("probe should not import credentials, err = %v", err)
	}
}

func TestProcessEnvUsesJazHomeForClaudeCode(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, "claude-config")
	t.Setenv("HOME", home)
	t.Setenv("ANTHROPIC_APIKEY", "host-anthropic-key")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "host-auth-token")
	t.Setenv("CLAUDE_CODE_EXECUTABLE", "/usr/local/bin/claude")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "setup-token")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "0")
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("USER", "wins")

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{})

	wantHome := filepath.Join(root, "acp", "home")
	if env["HOME"] != wantHome {
		t.Fatalf("HOME = %q, want %q", env["HOME"], wantHome)
	}
	if env["ANTHROPIC_API_KEY"] != "host-anthropic-key" {
		t.Fatalf("ANTHROPIC_API_KEY was not preserved and normalized")
	}
	if _, ok := env["ANTHROPIC_APIKEY"]; ok {
		t.Fatal("ANTHROPIC_APIKEY alias leaked into subprocess env")
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "host-auth-token" {
		t.Fatalf("ANTHROPIC_AUTH_TOKEN was not preserved")
	}
	if env["CLAUDE_CODE_EXECUTABLE"] != "/usr/local/bin/claude" {
		t.Fatalf("CLAUDE_CODE_EXECUTABLE = %q", env["CLAUDE_CODE_EXECUTABLE"])
	}
	if env["CLAUDE_CODE_OAUTH_TOKEN"] != "setup-token" {
		t.Fatalf("CLAUDE_CODE_OAUTH_TOKEN was not preserved")
	}
	if env["CLAUDE_CODE_USE_VERTEX"] != "0" {
		t.Fatalf("CLAUDE_CODE_USE_VERTEX = %q", env["CLAUDE_CODE_USE_VERTEX"])
	}
	wantConfigDir := filepath.Join(root, "acp", "claude")
	if env["CLAUDE_CONFIG_DIR"] != wantConfigDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", env["CLAUDE_CONFIG_DIR"], wantConfigDir)
	}
	if env["USER"] != "wins" {
		t.Fatalf("USER = %q, want wins", env["USER"])
	}
	if !strings.HasPrefix(env["TMPDIR"], filepath.Join(root, "acp")) || !strings.HasPrefix(env["npm_config_cache"], filepath.Join(root, "acp")) {
		t.Fatalf("expected claude temp/cache under jaz root: %#v", env)
	}
}

func TestProcessEnvHonorsConfiguredClaudeHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{
		Env: map[string]string{"HOME": home},
	})

	if env["HOME"] != home {
		t.Fatalf("HOME = %q, want configured claude home %q", env["HOME"], home)
	}
	if !strings.HasPrefix(env["TMPDIR"], filepath.Join(root, "acp")) || !strings.HasPrefix(env["npm_config_cache"], filepath.Join(root, "acp")) {
		t.Fatalf("expected claude temp/cache under jaz root: %#v", env)
	}
}

func TestProcessEnvUsesJazHomeForGrok(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XAI_APIKEY", "host-xai-key")
	t.Setenv("USER", "wins")

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("grok", AgentConfig{})

	wantHome := filepath.Join(root, "acp", "home")
	if env["HOME"] != wantHome {
		t.Fatalf("HOME = %q, want %q", env["HOME"], wantHome)
	}
	if env["XAI_API_KEY"] != "host-xai-key" {
		t.Fatalf("XAI_API_KEY was not preserved and normalized")
	}
	if _, ok := env["XAI_APIKEY"]; ok {
		t.Fatal("XAI_APIKEY alias leaked into subprocess env")
	}
	if env["USER"] != "wins" {
		t.Fatalf("USER = %q, want wins", env["USER"])
	}
	if !strings.HasPrefix(env["TMPDIR"], filepath.Join(root, "acp")) || !strings.HasPrefix(env["npm_config_cache"], filepath.Join(root, "acp")) {
		t.Fatalf("expected grok temp/cache under jaz root: %#v", env)
	}
}

func TestProcessEnvNeverLeaksAPIKeysToCodex(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("OPENAI_APIKEY", "openai-key")

	env := NewManager(nil, Config{
		Root: t.TempDir(),
		Env: map[string]string{
			"OPENAI_API_KEY":     "configured-openai-key",
			"OPENROUTER_APIKEY":  "configured-openrouter-key",
			"CODEX_API_KEY":      "configured-codex-key",
			"CODEX_ACCESS_TOKEN": "configured-access-token",
		},
	}, nil).processEnv("codex", AgentConfig{})

	for _, key := range []string{"OPENAI_API_KEY", "OPENROUTER_APIKEY", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN"} {
		if env[key] != "" {
			t.Fatalf("%s leaked into codex subprocess env", key)
		}
	}
	if env["CODEX_HOME"] == "" || !fileExists(filepath.Join(env["CODEX_HOME"], "auth.json")) {
		t.Fatalf("codex oauth auth was not prepared: %#v", env)
	}
}

func TestProbeReadinessRequiresCodexOAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	exe := testExecutable(t)

	ready := ProbeReadiness(AgentCodex, AgentConfig{Command: exe}, t.TempDir(), nil)
	if ready.Available || !strings.Contains(ready.Reason, "Codex OAuth login") {
		t.Fatalf("ready = %#v", ready)
	}

	codexHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ready = ProbeReadiness(AgentCodex, AgentConfig{Command: exe}, t.TempDir(), map[string]string{"CODEX_HOME": codexHome})
	if !ready.Available {
		t.Fatalf("codex should be ready with oauth auth: %#v", ready)
	}
}

func TestProbeReadinessRequiresClaudeExecutable(t *testing.T) {
	exe := testExecutable(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PATH", filepath.Dir(exe))

	ready := ProbeReadiness(AgentClaude, AgentConfig{Command: exe}, t.TempDir(), nil)
	if ready.Available || !strings.Contains(ready.Reason, "Claude Code executable") {
		t.Fatalf("ready = %#v", ready)
	}

	ready = ProbeReadiness(AgentClaude, AgentConfig{Command: exe}, t.TempDir(), map[string]string{"CLAUDE_CODE_EXECUTABLE": exe})
	if ready.Available || !strings.Contains(ready.Reason, "Claude login") {
		t.Fatalf("ready = %#v", ready)
	}

	ready = ProbeReadiness(AgentClaude, AgentConfig{Command: exe}, t.TempDir(), map[string]string{
		"CLAUDE_CODE_EXECUTABLE":  exe,
		"CLAUDE_CODE_OAUTH_TOKEN": "setup-token",
	})
	if !ready.Available {
		t.Fatalf("claude should be ready with executable and auth: %#v", ready)
	}
}

func TestProbeReadinessRequiresGrokAuth(t *testing.T) {
	exe := testExecutable(t)
	home := t.TempDir()

	ready := ProbeReadiness(AgentGrok, AgentConfig{Command: exe}, t.TempDir(), map[string]string{"HOME": home})
	if ready.Available || !strings.Contains(ready.Reason, "Grok login") {
		t.Fatalf("ready = %#v", ready)
	}

	if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grok", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ready = ProbeReadiness(AgentGrok, AgentConfig{Command: exe}, t.TempDir(), map[string]string{"HOME": home})
	if !ready.Available {
		t.Fatalf("grok should be ready with cached token: %#v", ready)
	}
}

func TestAutoAuthMethodSelectsConfiguredEnvVarForGenericAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	method, missing := autoAuthMethod("fake", codexInitializeAuthMethods(), map[string]string{"OPENAI_API_KEY": "key"})

	if method != "openai-api-key" || len(missing) != 0 {
		t.Fatalf("method=%q missing=%v", method, missing)
	}
}

func TestAutoAuthMethodDoesNotUseAPIKeyForCodex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	method, missing := autoAuthMethod("codex", codexInitializeAuthMethods(), map[string]string{"OPENAI_API_KEY": "key"})

	if method != "" || strings.Join(missing, ",") != "Codex OAuth login at ~/.codex/auth.json" {
		t.Fatalf("method=%q missing=%v", method, missing)
	}
}

func TestAutoAuthMethodPrefersCodexOAuth(t *testing.T) {
	codexHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	method, missing := autoAuthMethod("codex", codexInitializeAuthMethods(), map[string]string{
		"CODEX_HOME":     codexHome,
		"OPENAI_API_KEY": "key",
	})

	if method != "chatgpt" || len(missing) != 0 {
		t.Fatalf("method=%q missing=%v", method, missing)
	}
}

func TestAutoAuthMethodPrefersGrokAPIKeyWhenAdvertised(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	method, missing := autoAuthMethod("grok", grokInitializeAuthMethods(), map[string]string{"XAI_API_KEY": "key"})

	if method != "xai.api_key" || len(missing) != 0 {
		t.Fatalf("method=%q missing=%v", method, missing)
	}
}

func TestAutoAuthMethodUsesGrokCachedToken(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grok", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	method, missing := autoAuthMethod("grok", grokInitializeAuthMethods(), map[string]string{"HOME": home})

	if method != "cached_token" || len(missing) != 0 {
		t.Fatalf("method=%q missing=%v", method, missing)
	}
}

func TestAutoAuthMethodReportsMissingGrokAuth(t *testing.T) {
	home := t.TempDir()
	method, missing := autoAuthMethod("grok", grokInitializeAuthMethods(), map[string]string{"HOME": home})

	if method != "" || strings.Join(missing, ",") != "Grok login at "+filepath.Join(home, ".grok", "auth.json")+" or XAI_API_KEY" {
		t.Fatalf("method=%q missing=%v", method, missing)
	}
}

func TestProcessCommandAddsGrokReasoningEffortArg(t *testing.T) {
	_, args := processCommand("grok", AgentConfig{
		Command:         "grok",
		Args:            []string{"--no-auto-update", "agent", "--no-leader", "stdio"},
		ReasoningEffort: "high",
	})
	want := "--no-auto-update agent --no-leader --always-approve --reasoning-effort high stdio"
	if strings.Join(args, " ") != want {
		t.Fatalf("args = %q, want %q", strings.Join(args, " "), want)
	}
}

func TestProcessCommandDoesNotDuplicateGrokAlwaysApproveArg(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "already always approve",
			args: []string{"agent", "--always-approve", "stdio"},
			want: "agent --always-approve stdio",
		},
		{
			name: "explicit permission mode",
			args: []string{"agent", "--permission-mode", "ask", "stdio"},
			want: "agent --permission-mode ask stdio",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, args := processCommand("grok", AgentConfig{
				Command: "grok",
				Args:    tc.args,
			})
			if strings.Join(args, " ") != tc.want {
				t.Fatalf("args = %q, want %q", strings.Join(args, " "), tc.want)
			}
		})
	}
}

func TestProcessCommandDoesNotDuplicateGrokReasoningEffortArg(t *testing.T) {
	_, args := processCommand("grok", AgentConfig{
		Command:         "grok",
		Args:            []string{"agent", "--reasoning-effort=low", "stdio"},
		ReasoningEffort: "high",
	})
	if strings.Join(args, " ") != "agent --reasoning-effort=low --always-approve stdio" {
		t.Fatalf("args = %#v", args)
	}
}

func TestProcessCommandLeavesNonGrokCommandAlone(t *testing.T) {
	_, args := processCommand("grok", AgentConfig{
		Command:         os.Args[0],
		Args:            []string{"-test.run=TestFakeACPAgentProcess"},
		ReasoningEffort: "high",
	})
	if strings.Join(args, " ") != "-test.run=TestFakeACPAgentProcess" {
		t.Fatalf("args = %#v", args)
	}
}

func TestAutoAuthMethodReportsMissingEnvVars(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	method, missing := autoAuthMethod("codex", codexInitializeAuthMethods(), nil)

	if method != "" || strings.Join(missing, ",") != "Codex OAuth login at ~/.codex/auth.json" {
		t.Fatalf("method=%q missing=%v", method, missing)
	}
}

func codexInitializeAuthMethods() []byte {
	return []byte(`{
		"authMethods": [
			{"id": "chatgpt", "name": "Login with ChatGPT"},
			{"type": "env_var", "id": "openai-api-key", "vars": [{"name": "OPENAI_API_KEY"}]}
		]
	}`)
}

func grokInitializeAuthMethods() []byte {
	return []byte(`{
		"authMethods": [
			{"id": "cached_token", "name": "cached_token"},
			{"id": "grok.com", "name": "Grok"},
			{"id": "xai.api_key", "name": "XAI API Key"}
		]
	}`)
}

func testExecutable(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}
