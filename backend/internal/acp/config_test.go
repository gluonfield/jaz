package acp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProcessEnvIsMinimalAndCanonical(t *testing.T) {
	clearHostEnv(t)
	t.Setenv("PATH", "/bin")
	t.Setenv("CODEX_HOME", "/host/codex")
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
	if env["EXPLICIT"] != "yes" {
		t.Fatalf("explicit env missing: %#v", env)
	}
	assertEnv(t, env, map[string]string{
		"PATH":              "/bin",
		"EXPLICIT":          "yes",
		"OPENAI_API_KEY":    "openai-key",
		"ANTHROPIC_API_KEY": "anthropic-key",
	})
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
	clearHostEnv(t)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PATH", "/bin")

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("codex", AgentConfig{})

	want := filepath.Join(home, ".codex")
	if env["CODEX_HOME"] != want {
		t.Fatalf("CODEX_HOME = %q", env["CODEX_HOME"])
	}
	if _, ok := env["HOME"]; ok {
		t.Fatal("codex subprocess HOME should not be set")
	}
	assertEnv(t, env, map[string]string{
		"PATH":       "/bin",
		"CODEX_HOME": want,
	})
	if fileExists(filepath.Join(root, "acp", "codex-home", "auth.json")) {
		t.Fatal("codex auth should not be silently imported into the Jaz profile")
	}
}

func TestProcessEnvPreparedReportsProfilePreparationFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "acp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "acp", "codex-home"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("codex", AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeJazProfile},
	})
	if err == nil || !strings.Contains(err.Error(), "prepare codex profile") {
		t.Fatalf("err = %v, want codex profile preparation error", err)
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

func TestProbeAgentAuthDetectsCodexKeyringProfile(t *testing.T) {
	home := t.TempDir()
	codexHome := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(`cli_auth_credentials_store = "keyring"`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	status := ProbeAgentAuth(AgentCodex, AgentConfig{}, t.TempDir(), nil)
	if !status.Authenticated || status.AuthEvidence != "keyring_config" {
		t.Fatalf("status = %#v, want keyring-backed auth", status)
	}
	if status.AuthSource != AuthModeExistingCLI || status.AuthPath != codexHome {
		t.Fatalf("status = %#v, want existing CLI profile at %s", status, codexHome)
	}
}

func TestProbeAgentAuthDetectsClaudeJSONProfile(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	configDir := filepath.Join(home, "claude-config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".claude.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	status := ProbeAgentAuth(AgentClaude, AgentConfig{}, t.TempDir(), nil)
	if !status.Authenticated || status.AuthEvidence != "claude_json" {
		t.Fatalf("status = %#v, want .claude.json auth", status)
	}
	if status.StoragePath != filepath.Join(configDir, ".claude.json") {
		t.Fatalf("storage path = %q, want .claude.json", status.StoragePath)
	}
}

func TestProcessEnvUsesJazConfigForClaudeCode(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	configDir := filepath.Join(home, "claude-config")
	t.Setenv("PATH", "/bin")
	t.Setenv("HOME", home)
	t.Setenv("ANTHROPIC_APIKEY", "host-anthropic-key")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "host-auth-token")
	t.Setenv("CLAUDE_CODE_EXECUTABLE", "/usr/local/bin/claude")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "setup-token")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "0")
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("USER", "wins")

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeJazProfile},
	})

	if _, ok := env["HOME"]; ok {
		t.Fatalf("claude subprocess HOME should not be set: %#v", env)
	}
	if env["ANTHROPIC_API_KEY"] != "" {
		t.Fatalf("ANTHROPIC_API_KEY leaked into claude subprocess env")
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
	assertEnv(t, env, map[string]string{
		"PATH":                    "/bin",
		"ANTHROPIC_AUTH_TOKEN":    "host-auth-token",
		"CLAUDE_CODE_EXECUTABLE":  "/usr/local/bin/claude",
		"CLAUDE_CODE_OAUTH_TOKEN": "setup-token",
		"CLAUDE_CODE_USE_VERTEX":  "0",
		"CLAUDE_CONFIG_DIR":       wantConfigDir,
		"USER":                    "wins",
	})
}

func TestProcessEnvFindsClaudeCodeFromLoginShell(t *testing.T) {
	clearHostEnv(t)
	claude := testExecutable(t)
	shell := filepath.Join(t.TempDir(), "shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf '%s\n' \"$JAZ_TEST_CLAUDE\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/bin")
	t.Setenv("SHELL", shell)
	t.Setenv("JAZ_TEST_CLAUDE", claude)

	env := NewManager(nil, Config{Root: t.TempDir()}, nil).probeEnv("claude", AgentConfig{})

	if env["CLAUDE_CODE_EXECUTABLE"] != claude {
		t.Fatalf("CLAUDE_CODE_EXECUTABLE = %q, want %q", env["CLAUDE_CODE_EXECUTABLE"], claude)
	}
}

func TestProcessEnvIgnoresConfiguredClaudeHome(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	t.Setenv("PATH", "/bin")
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{
		Env: map[string]string{"HOME": home},
	})

	if _, ok := env["HOME"]; ok {
		t.Fatalf("claude subprocess HOME should not be set: %#v", env)
	}
	wantConfigDir := filepath.Join(root, "acp", "claude")
	if env["CLAUDE_CONFIG_DIR"] != wantConfigDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", env["CLAUDE_CONFIG_DIR"], wantConfigDir)
	}
	assertEnv(t, env, map[string]string{
		"PATH":              "/bin",
		"CLAUDE_CONFIG_DIR": wantConfigDir,
	})
}

func TestProcessEnvDoesNotSetHomeForGrok(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	t.Setenv("PATH", "/bin")
	t.Setenv("HOME", home)
	t.Setenv("XAI_APIKEY", "host-xai-key")
	t.Setenv("USER", "wins")

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("grok", AgentConfig{})

	if _, ok := env["HOME"]; ok {
		t.Fatalf("grok subprocess HOME should not be set: %#v", env)
	}
	if env["XAI_API_KEY"] != "" {
		t.Fatalf("XAI_API_KEY leaked into grok subprocess env")
	}
	if _, ok := env["XAI_APIKEY"]; ok {
		t.Fatal("XAI_APIKEY alias leaked into subprocess env")
	}
	if env["USER"] != "wins" {
		t.Fatalf("USER = %q, want wins", env["USER"])
	}
	if _, err := os.Stat(filepath.Join(root, "acp", "home")); !os.IsNotExist(err) {
		t.Fatalf("grok fake home should not be created, err = %v", err)
	}
	assertEnv(t, env, map[string]string{
		"PATH": "/bin",
		"USER": "wins",
	})
}

func TestProcessEnvNeverLeaksAPIKeysToCodex(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/bin")
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
	assertEnv(t, env, map[string]string{
		"PATH":       "/bin",
		"CODEX_HOME": filepath.Join(home, ".codex"),
	})
}

func TestProcessEnvMapsExplicitACPAPIKeysOnlyWhenNeeded(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("JAZ_ACP_CODEX_API_KEY", "codex-key")
	t.Setenv("JAZ_ACP_CLAUDE_API_KEY", "claude-key")
	t.Setenv("JAZ_ACP_GROK_API_KEY", "grok-key")

	manager := NewManager(nil, Config{Root: root}, nil)
	codexEnv := manager.processEnv("codex", AgentConfig{})
	if codexEnv["OPENAI_API_KEY"] != "codex-key" || codexEnv["JAZ_ACP_CODEX_API_KEY"] != "" {
		t.Fatalf("codex explicit key not mapped cleanly: %#v", codexEnv)
	}
	claudeEnv := manager.processEnv("claude", AgentConfig{})
	if claudeEnv["ANTHROPIC_API_KEY"] != "claude-key" || claudeEnv["JAZ_ACP_CLAUDE_API_KEY"] != "" {
		t.Fatalf("claude explicit key not mapped cleanly: %#v", claudeEnv)
	}
	grokEnv := manager.processEnv("grok", AgentConfig{})
	if grokEnv["XAI_API_KEY"] != "grok-key" || grokEnv["JAZ_ACP_GROK_API_KEY"] != "" {
		t.Fatalf("grok explicit key not mapped cleanly: %#v", grokEnv)
	}
}

func TestProcessEnvPrefersAccountAuthOverExplicitAPIKeys(t *testing.T) {
	root := t.TempDir()
	codexHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("JAZ_ACP_CODEX_API_KEY", "codex-key")

	env := NewManager(nil, Config{Root: root}, nil).processEnv("codex", AgentConfig{})
	if env["OPENAI_API_KEY"] != "" {
		t.Fatalf("codex api key should not be injected when oauth is available: %#v", env)
	}
}

func TestProbeReadinessAllowsCodexOAuthOrExplicitAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	exe := testExecutable(t)

	ready := ProbeReadiness(AgentCodex, AgentConfig{Command: exe}, t.TempDir(), nil)
	if ready.Available || !strings.Contains(ready.Reason, "Codex login") {
		t.Fatalf("ready = %#v", ready)
	}

	ready = ProbeReadiness(AgentCodex, AgentConfig{Command: exe}, t.TempDir(), map[string]string{"JAZ_ACP_CODEX_API_KEY": "key"})
	if !ready.Available {
		t.Fatalf("codex should be ready with explicit api key: %#v", ready)
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
	t.Setenv("HOME", home)

	ready := ProbeReadiness(AgentGrok, AgentConfig{Command: exe}, t.TempDir(), nil)
	if ready.Available || !strings.Contains(ready.Reason, "Grok login") {
		t.Fatalf("ready = %#v", ready)
	}

	if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grok", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ready = ProbeReadiness(AgentGrok, AgentConfig{Command: exe}, t.TempDir(), nil)
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

func TestAutoAuthMethodUsesAPIKeyForCodexWhenOAuthMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	method, missing := autoAuthMethod("codex", codexInitializeAuthMethods(), map[string]string{"OPENAI_API_KEY": "key"})

	if method != "openai-api-key" || len(missing) != 0 {
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

func TestAutoAuthMethodPrefersGrokCachedTokenOverAPIKey(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grok", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	method, missing := autoAuthMethod("grok", grokInitializeAuthMethods(), map[string]string{
		"HOME":        home,
		"XAI_API_KEY": "key",
	})

	if method != "cached_token" || len(missing) != 0 {
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

	if method != "" || strings.Join(missing, ",") != "Grok login at "+filepath.Join(home, ".grok", "auth.json")+" or JAZ_ACP_GROK_API_KEY" {
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

	if method != "" || strings.Join(missing, ",") != "Codex OAuth login at ~/.codex/auth.json,JAZ_ACP_CODEX_API_KEY" {
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

func clearHostEnv(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		t.Setenv(key, "")
	}
}

func assertEnv(t *testing.T, got, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("env = %#v, want %#v", got, want)
	}
}
