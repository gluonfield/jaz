package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
)

type testPrompt string

func (p testPrompt) ACPPrompt(string) (string, error) { return string(p), nil }
func (p testPrompt) SkillsPromptForWorkspace(string) (string, error) {
	return string(p), nil
}

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

func TestMergeAgentsPreservesCapabilitiesOnPartialOverride(t *testing.T) {
	merged := MergeAgents(BuiltinAgents(), map[string]AgentConfig{
		AgentOpenCode: {
			Command: "opencode",
			Model:   "anthropic/claude-sonnet-4.5",
			Env:     map[string]string{"EXTRA": "yes"},
		},
	})
	got, ok := merged.Agent(AgentOpenCode)
	if !ok {
		t.Fatal("opencode missing")
	}
	if got.Command != "opencode" || len(got.Args) != 0 {
		t.Fatalf("command override = %q %#v", got.Command, got.Args)
	}
	if got.ProviderMode != AgentProviderModeAgentDefaults ||
		got.ModelProviderCapability != modelprovider.CapabilityOpenCode ||
		got.ModelProvider != modelprovider.ProviderOpenRouter {
		t.Fatalf("capabilities not preserved: %#v", got)
	}
	if got.Model != "anthropic/claude-sonnet-4.5" || got.Env["EXTRA"] != "yes" {
		t.Fatalf("override fields not applied: %#v", got)
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

func TestProbeAgentAuthIgnoresClaudeSettingsOnlyJSONProfile(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	configDir := filepath.Join(home, "claude-config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".claude.json"), []byte(`{"hasCompletedOnboarding":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	status := ProbeAgentAuth(AgentClaude, AgentConfig{}, t.TempDir(), nil)
	if status.Authenticated {
		t.Fatalf("status = %#v, want settings-only .claude.json ignored", status)
	}
}

func TestProbeAgentAuthDetectsClaudeJSONProfile(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	configDir := filepath.Join(home, "claude-config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"account-id"}}`), 0o600); err != nil {
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

func TestProcessEnvUsesJazConfigForClaudeCodeWithoutHostAuthTokens(t *testing.T) {
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
	env := NewManager(nil, Config{
		Root: root,
		Env: map[string]string{
			"ANTHROPIC_AUTH_TOKEN": "configured-auth-token",
			"CLAUDE_CONFIG_DIR":    filepath.Join(home, "configured-claude"),
		},
	}, nil).processEnv("claude", AgentConfig{
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
	if env["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Fatalf("ANTHROPIC_AUTH_TOKEN leaked into Jaz-profile claude subprocess env")
	}
	if env["CLAUDE_CODE_EXECUTABLE"] != "/usr/local/bin/claude" {
		t.Fatalf("CLAUDE_CODE_EXECUTABLE = %q", env["CLAUDE_CODE_EXECUTABLE"])
	}
	if env["CLAUDE_CODE_OAUTH_TOKEN"] != "" {
		t.Fatalf("CLAUDE_CODE_OAUTH_TOKEN leaked into Jaz-profile claude subprocess env")
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
		"PATH":                   "/bin",
		"CLAUDE_CODE_EXECUTABLE": "/usr/local/bin/claude",
		"CLAUDE_CODE_USE_VERTEX": "0",
		"CLAUDE_CONFIG_DIR":      wantConfigDir,
		"USER":                   "wins",
	})
}

func TestProcessEnvPreparedSyncsClaudeSkills(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	writeACPTestSkill(t, root, "alpha")
	t.Setenv("CLAUDE_CODE_EXECUTABLE", "/usr/local/bin/claude")

	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("claude", AgentConfig{
		Auth: AgentAuthConfig{Mode: AuthModeJazProfile},
	})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(env["CLAUDE_CONFIG_DIR"], "skills", "alpha", "SKILL.md")
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "Alpha skill") {
		t.Fatalf("claude skill copy = %q, %v", data, err)
	}
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
	clearHostEnv(t)
	root := t.TempDir()
	t.Setenv("PATH", "/bin")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("JAZ_ACP_CODEX_API_KEY", "codex-key")
	t.Setenv("JAZ_ACP_CLAUDE_API_KEY", "claude-key")
	t.Setenv("JAZ_ACP_GROK_API_KEY", "grok-key")
	t.Setenv("JAZ_ACP_OPENCODE_API_KEY", "opencode-key")

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
	openCodeEnv := manager.processEnv("opencode", AgentConfig{})
	if openCodeEnv["OPENROUTER_API_KEY"] != "opencode-key" || openCodeEnv["JAZ_ACP_OPENCODE_API_KEY"] != "" {
		t.Fatalf("opencode explicit key not mapped cleanly: %#v", openCodeEnv)
	}
	if openCodeEnv["OPENCODE_CONFIG_DIR"] == "" {
		t.Fatalf("opencode config dir missing: %#v", openCodeEnv)
	}
}

func TestProcessEnvPassesModelProviderKeysToOpenCode(t *testing.T) {
	root := t.TempDir()
	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{
		"OPENROUTER_API_KEY": "openrouter-key",
		"OPENAI_API_KEY":     "openai-key",
	}); err != nil {
		t.Fatal(err)
	}

	env := NewManager(nil, Config{Root: root}, nil).processEnv("opencode", AgentConfig{})

	if env["OPENROUTER_API_KEY"] != "openrouter-key" || env["OPENAI_API_KEY"] != "openai-key" {
		t.Fatalf("provider keys not passed to opencode: %#v", env)
	}
	if env["OPENCODE_CONFIG_DIR"] != filepath.Join(root, "acp", "opencode") {
		t.Fatalf("OPENCODE_CONFIG_DIR = %q", env["OPENCODE_CONFIG_DIR"])
	}
}

func TestProcessEnvWritesOpenCodeInstructions(t *testing.T) {
	root := t.TempDir()
	env, err := NewManager(nil, Config{
		Root:         root,
		SystemPrompt: testPrompt("jaz instructions"),
	}, nil).processEnvPrepared("opencode", AgentConfig{})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "acp", "opencode", "jaz-instructions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "jaz instructions\n" {
		t.Fatalf("instructions = %q", data)
	}
	if !strings.Contains(env["OPENCODE_CONFIG_CONTENT"], path) {
		t.Fatalf("OPENCODE_CONFIG_CONTENT = %q", env["OPENCODE_CONFIG_CONTENT"])
	}
}

func TestProcessEnvDoesNotOverrideDefaultOpenCodeProvider(t *testing.T) {
	root := t.TempDir()
	env, err := NewManager(nil, Config{
		Root: root,
		Providers: map[string]modelprovider.ModelProviderConfig{
			"openrouter": {
				Type:    "openrouter",
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  "openrouter-key",
			},
		},
		SystemPrompt: testPrompt("jaz instructions"),
	}, nil).processEnvPrepared("opencode", AgentConfig{
		Model: "openrouter/openai/gpt-5.4-mini",
	})
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		Provider map[string]any `json:"provider"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatalf("config content = %q: %v", env["OPENCODE_CONFIG_CONTENT"], err)
	}
	if len(content.Provider) != 0 {
		t.Fatalf("default openrouter provider should not be overridden: %#v", content.Provider)
	}
}

func TestProcessEnvWritesOpenCodeProviderConfig(t *testing.T) {
	root := t.TempDir()
	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("opencode", AgentConfig{
		Model: "ollama/llama3.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		Provider map[string]struct {
			API    string `json:"api"`
			NPM    string `json:"npm"`
			Models map[string]struct {
				ID string `json:"id"`
			} `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatalf("config content = %q: %v", env["OPENCODE_CONFIG_CONTENT"], err)
	}
	ollama := content.Provider["ollama"]
	if ollama.API != "http://localhost:11434/v1" || ollama.NPM != "@ai-sdk/openai-compatible" {
		t.Fatalf("ollama provider config = %#v", ollama)
	}
	if _, ok := ollama.Models["llama3.2"]; !ok {
		t.Fatalf("ollama models = %#v", ollama.Models)
	}
}

func TestProcessEnvUsesSplitOpenCodeProviderModel(t *testing.T) {
	root := t.TempDir()
	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("opencode", AgentConfig{
		ProviderMode:  AgentProviderModeAgentDefaults,
		ModelProvider: "ollama",
		Model:         "llama3.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		Provider map[string]struct {
			Models map[string]struct {
				ID string `json:"id"`
			} `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatalf("config content = %q: %v", env["OPENCODE_CONFIG_CONTENT"], err)
	}
	if _, ok := content.Provider["ollama"].Models["llama3.2"]; !ok {
		t.Fatalf("ollama models = %#v", content.Provider["ollama"].Models)
	}
}

func TestProcessEnvWritesCustomOpenCodeProviderConfig(t *testing.T) {
	root := t.TempDir()
	env, err := NewManager(nil, Config{
		Root: root,
		Providers: map[string]modelprovider.ModelProviderConfig{
			"internal": {
				Type:    "openai-compatible",
				Label:   "Internal",
				BaseURL: "https://llm.internal/v1",
				APIKey:  "internal-key",
			},
		},
	}, nil).processEnvPrepared("opencode", AgentConfig{Model: "internal/chat"})
	if err != nil {
		t.Fatal(err)
	}
	if env["JAZ_PROVIDER_INTERNAL_API_KEY"] != "internal-key" {
		t.Fatalf("custom provider key not mapped: %#v", env)
	}
	var content struct {
		Provider map[string]struct {
			API    string   `json:"api"`
			Env    []string `json:"env"`
			Models map[string]struct {
				ID string `json:"id"`
			} `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatalf("config content = %q: %v", env["OPENCODE_CONFIG_CONTENT"], err)
	}
	internal := content.Provider["internal"]
	if internal.API != "https://llm.internal/v1" || strings.Join(internal.Env, ",") != "JAZ_PROVIDER_INTERNAL_API_KEY" {
		t.Fatalf("internal provider config = %#v", internal)
	}
	if _, ok := internal.Models["chat"]; !ok {
		t.Fatalf("internal models = %#v", internal.Models)
	}
}

func TestProbeAgentAuthMatchesOpenCodeModelProvider(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()

	status := ProbeAgentAuth(AgentOpenCode, AgentConfig{Model: "openrouter/openai/gpt-5.4-mini"}, root, map[string]string{
		"OPENAI_API_KEY": "openai-key",
	})
	if status.Authenticated {
		t.Fatalf("openrouter model authenticated with openai key: %#v", status)
	}

	t.Setenv("JAZ_ACP_OPENCODE_API_KEY", "openrouter-key")
	status = ProbeAgentAuth(AgentOpenCode, AgentConfig{Model: "openrouter/openai/gpt-5.4-mini"}, root, nil)
	if !status.Authenticated || status.AuthEvidence != "api_key_env" {
		t.Fatalf("openrouter model did not authenticate with explicit opencode key: %#v", status)
	}

	status = ProbeAgentAuth(AgentOpenCode, AgentConfig{Model: "openai/gpt-5.4-mini"}, root, map[string]string{
		"OPENAI_API_KEY": "openai-key",
	})
	if !status.Authenticated || status.AuthEvidence != "openai_api_key_env" {
		t.Fatalf("openai model did not authenticate with openai key: %#v", status)
	}
}

func TestProbeAgentAuthMatchesCustomOpenCodeProvider(t *testing.T) {
	root := t.TempDir()
	providers := map[string]modelprovider.ModelProviderConfig{
		"internal": {
			Type:    "openai-compatible",
			BaseURL: "https://llm.internal/v1",
			APIKey:  "internal-key",
		},
	}

	status := ProbeAgentAuthWithProviders(AgentOpenCode, AgentConfig{Model: "internal/chat"}, root, nil, providers)
	if !status.Authenticated || status.AuthKind != AuthKindAPIKey {
		t.Fatalf("custom provider did not authenticate with configured key: %#v", status)
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
	t.Setenv("CLAUDE_CODE_EXECUTABLE", "")
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

func writeACPTestSkill(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, "skills", name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: Alpha skill\n---\nbody"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
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
