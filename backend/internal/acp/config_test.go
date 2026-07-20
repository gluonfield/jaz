package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/managedtool"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/promptmodule"
	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/testexec"
)

type testPrompt string

func (p testPrompt) ACPPromptForContext(_ context.Context, _, _ string) (string, error) {
	return string(p), nil
}

type cwdPrompt struct{}

func (cwdPrompt) ACPPromptForContext(_ context.Context, cwd, _ string) (string, error) {
	return "cwd=" + cwd, nil
}

type platformPrompt struct{}

func (platformPrompt) ACPPromptForContext(ctx context.Context, _, _ string) (string, error) {
	return "platform=" + sessioncontext.ClientPlatform(ctx), nil
}

type modulePrompt struct{}

const workerConnectionsPrompt = "## connections\n\nConnected accounts and agent-relevant memory paths:\n- Telegram: personal (42)\n  - `sources/chat/telegram/42/contacts.md` (memory_page): Clean contact index.\n  - `sources/chat/telegram/42/conversations/` (memory_prefix): Materialized chat days."

const workerMemoryPrompt = "## memory\n\nJaz has persistent markdown memory (jazmem) at /tmp/jaz/memory.\n\nCore memory paths:\n- `LONG_TERM.md`: stable identity, goals, preferences, and key relationships.\n- `SHORT_TERM.md`: current focus, active projects, and open loops.\n- `sources/`: cleaned source pages from providers or agents.\n\n## memory/LONG_TERM.md\n\n- long\n\n## memory/SHORT_TERM.md\n\n- short\n\n## memory/daily/2026-06-30.md\n\n- today"

func (modulePrompt) ACPPromptForContext(context.Context, string, string) (string, error) {
	return "full platform prompt", nil
}

func (modulePrompt) PromptModulesForContext(_ context.Context, opts PromptModuleOptions) (promptmodule.Modules, error) {
	out := promptmodule.Modules{}
	if opts.Connections {
		out = out.Append(workerConnectionsPrompt)
	}
	return out.Append(workerMemoryPrompt), nil
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

func TestCodexBuiltinAgentUsesManagedAdapter(t *testing.T) {
	cfg := codexBuiltinAgent()
	if cfg.Command != "" || cfg.ManagedAdapter != "codex" {
		t.Fatalf("cfg = %#v, want managed adapter", cfg)
	}
	if cfg.Model != modelprovider.OpenAIModelGPT56Sol {
		t.Fatalf("model = %q, want %q", cfg.Model, modelprovider.OpenAIModelGPT56Sol)
	}
	args := strings.Join(cfg.ManagedAdapterArgs, "\n")
	if !strings.Contains(args, `sandbox_mode="danger-full-access"`) {
		t.Fatalf("managed args = %#v", cfg.ManagedAdapterArgs)
	}
}

func TestAntigravityBuiltinAgentUsesManagedAdapter(t *testing.T) {
	cfg := BuiltinAgents()[AgentAntigravity]
	if cfg.Command != "" || cfg.ManagedAdapter != "antigravity" {
		t.Fatalf("cfg = %#v, want managed adapter", cfg)
	}
	if !reflect.DeepEqual(cfg.ManagedAdapterArgs, []string{"--auth=auto", "--dangerously-skip-permissions"}) {
		t.Fatalf("managed args = %#v", cfg.ManagedAdapterArgs)
	}
	if cfg.ManagedTool != "antigravity-cli" {
		t.Fatalf("managed tool = %q", cfg.ManagedTool)
	}
	if cfg.ManagedToolAdapterArg != "--agy" {
		t.Fatalf("managed tool adapter arg = %q", cfg.ManagedToolAdapterArg)
	}
}

func TestLaunchCommandWrapsWindowsCommandScripts(t *testing.T) {
	command, args := resolvedLaunchCommand("windows", "npx.cmd", `C:\Program Files\nodejs\npx.cmd`, []string{"--version"})
	if command != "cmd.exe" {
		t.Fatalf("command = %q", command)
	}
	want := []string{"/d", "/s", "/c", "call", "npx.cmd", "--version"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestLaunchCommandWrapsAbsoluteWindowsCommandScripts(t *testing.T) {
	resolved := `C:\Program Files\nodejs\npx.cmd`
	command, args := resolvedLaunchCommand("windows", resolved, resolved, []string{"--version"})
	if command != "cmd.exe" {
		t.Fatalf("command = %q", command)
	}
	want := []string{"/d", "/s", "/c", "call", resolved, "--version"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestLaunchCommandLeavesNativeExecutablesAlone(t *testing.T) {
	command, args := resolvedLaunchCommand("windows", "node", `C:\Program Files\nodejs\node.exe`, []string{"--version"})
	if command != `C:\Program Files\nodejs\node.exe` {
		t.Fatalf("command = %q", command)
	}
	if !reflect.DeepEqual(args, []string{"--version"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestAddCommandDirToPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH handling is platform-specific")
	}
	env := map[string]string{"PATH": "/usr/bin:/bin"}

	addCommandDirToPath(env, "/opt/homebrew/bin/npx")
	addCommandDirToPath(env, "/opt/homebrew/bin/npm")

	if env["PATH"] != "/opt/homebrew/bin:/usr/bin:/bin" {
		t.Fatalf("PATH = %q", env["PATH"])
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
		{AgentKimi, map[string]any{"systemPrompt": "jaz prompt"}},
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

func TestSessionPromptMetaAppendsPerSessionExtension(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: testPrompt("base prompt")}}
	got, err := manager.sessionPromptMeta(context.Background(), AgentCodex, "", "", "", []string{"run context"})
	if err != nil {
		t.Fatal(err)
	}
	if got["systemPrompt"] != "base prompt\n\nrun context" {
		t.Fatalf("system prompt = %#v", got)
	}
}

func TestSessionPromptMetaUsesClientPlatformContext(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: platformPrompt{}}}
	got, err := manager.sessionPromptMeta(sessioncontext.WithClientPlatform(context.Background(), "mobile"), AgentCodex, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got["systemPrompt"] != "platform=mobile" {
		t.Fatalf("system prompt = %#v", got)
	}
}

func TestSessionPromptMetaAllowsExtensionWithoutBasePrompt(t *testing.T) {
	manager := &Manager{}
	got, err := manager.sessionPromptMeta(context.Background(), AgentCodex, "", "", "", []string{"run context"})
	if err != nil {
		t.Fatal(err)
	}
	if got["systemPrompt"] != "run context" {
		t.Fatalf("system prompt = %#v", got)
	}
}

func TestSessionPromptMetaSendsGrokExtensionsAsRules(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: testPrompt("jaz platform prompt")}}
	got, err := manager.sessionPromptMeta(context.Background(), AgentGrok, "", "widget", "", []string{"Scheduled Jaz loop run.\n\n## Board Widget Runtime\n\nPublish with visualise_publish_widget."})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["systemPrompt"]; ok {
		t.Fatalf("grok must not receive systemPrompt meta: %#v", got)
	}
	rules, ok := got["rules"].(string)
	if !ok {
		t.Fatalf("grok rules missing: %#v", got)
	}
	for _, want := range []string{"jaz platform prompt", "Scheduled Jaz loop run.", "## Board Widget Runtime", "visualise_publish_widget"} {
		if !strings.Contains(rules, want) {
			t.Fatalf("grok rules missing %q:\n%s", want, rules)
		}
	}
}

func TestSessionPromptMetaSkipsBasePromptForRestrictedWorker(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: testPrompt("jaz platform prompt")}}
	got, err := manager.sessionPromptMeta(context.Background(), AgentCodex, "", "", MCPServerPolicyBrowserWorker, []string{"browser worker prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if got["systemPrompt"] != "browser worker prompt" {
		t.Fatalf("system prompt = %#v", got)
	}
}

func TestSessionPromptMetaAddsRestrictedWorkerModules(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: modulePrompt{}}}
	for _, tc := range []struct {
		policy string
		worker string
		want   string
	}{
		{MCPServerPolicyMemorySearchWorker, "memory-search worker prompt", workerConnectionsPrompt + "\n\n" + workerMemoryPrompt + "\n\nmemory-search worker prompt"},
		{MCPServerPolicyMemorySourceWorker, "memory-source worker prompt", workerConnectionsPrompt + "\n\n" + workerMemoryPrompt + "\n\nmemory-source worker prompt"},
	} {
		got, err := manager.sessionPromptMeta(context.Background(), AgentCodex, "", "", tc.policy, []string{tc.worker})
		if err != nil {
			t.Fatal(err)
		}
		prompt := got["systemPrompt"].(string)
		if prompt != tc.want {
			t.Fatalf("system prompt = %#v", got)
		}
		for _, want := range []string{"## connections", "sources/chat/telegram/42/contacts.md", "sources/chat/telegram/42/conversations/", "## memory", "Core memory paths:", "## memory/LONG_TERM.md", "## memory/SHORT_TERM.md", "## memory/daily/2026-06-30.md"} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("%s worker prompt missing %q:\n%s", tc.policy, want, prompt)
			}
		}
		if strings.Contains(prompt, "/tmp/jaz/ingest") {
			t.Fatalf("restricted worker prompt must not include ingest paths:\n%s", prompt)
		}
		if strings.Contains(prompt, "full platform") {
			t.Fatalf("restricted worker received full platform prompt: %#v", got)
		}
	}

	got, err := manager.sessionPromptMeta(context.Background(), AgentCodex, "", "", MCPServerPolicyBrowserWorker, []string{"browser worker prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if got["systemPrompt"] != workerMemoryPrompt+"\n\nbrowser worker prompt" {
		t.Fatalf("browser worker prompt = %#v", got)
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
		got.ModelProviderCapability != modelprovider.CapabilityChatCompletions ||
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

func TestProcessEnvSetsCodexHomeAndHomeFromSystemLogin(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeCodexOAuth(t, filepath.Join(home, ".codex"))
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PATH", "/bin")

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("codex", AgentConfig{})

	want := filepath.Join(home, ".codex")
	if env["CODEX_HOME"] != want {
		t.Fatalf("CODEX_HOME = %q", env["CODEX_HOME"])
	}
	if env["HOME"] != home {
		t.Fatalf("HOME = %q, want %q", env["HOME"], home)
	}
	assertEnv(t, env, map[string]string{
		"PATH":       "/bin",
		"HOME":       home,
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
	writeCodexOAuth(t, filepath.Join(home, ".codex"))
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

	if env["HOME"] != home {
		t.Fatalf("HOME = %q, want %q", env["HOME"], home)
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
		"HOME":                   home,
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
	if runtime.GOOS == "windows" {
		t.Skip("login shell executable lookup is POSIX-only")
	}
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

func TestProcessEnvUsesHostHomeForClaude(t *testing.T) {
	clearHostEnv(t)
	configuredHome := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("PATH", "/bin")
	t.Setenv("HOME", hostHome)

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("claude", AgentConfig{
		Env: map[string]string{"HOME": configuredHome},
	})

	if env["HOME"] != hostHome {
		t.Fatalf("HOME = %q, want host HOME %q", env["HOME"], hostHome)
	}
	wantConfigDir := filepath.Join(root, "acp", "claude")
	if env["CLAUDE_CONFIG_DIR"] != wantConfigDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", env["CLAUDE_CONFIG_DIR"], wantConfigDir)
	}
	assertEnv(t, env, map[string]string{
		"HOME":              hostHome,
		"PATH":              "/bin",
		"CLAUDE_CONFIG_DIR": wantConfigDir,
	})
}

func TestProcessEnvUsesHostHomeForGrok(t *testing.T) {
	clearHostEnv(t)
	home := t.TempDir()
	t.Setenv("PATH", "/bin")
	t.Setenv("HOME", home)
	t.Setenv("XAI_APIKEY", "host-xai-key")
	t.Setenv("USER", "wins")

	root := t.TempDir()
	env := NewManager(nil, Config{Root: root}, nil).processEnv("grok", AgentConfig{})

	if env["HOME"] != home {
		t.Fatalf("HOME = %q, want %q", env["HOME"], home)
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
		"HOME": home,
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
	writeCodexOAuth(t, filepath.Join(home, ".codex"))
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
		"HOME":       home,
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

func TestProcessEnvBindsOnlySelectedModelProviderKeyToOpenCode(t *testing.T) {
	root := t.TempDir()
	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{
		"OPENROUTER_API_KEY": "openrouter-key",
		"OPENAI_API_KEY":     "openai-key",
	}); err != nil {
		t.Fatal(err)
	}

	env := NewManager(nil, Config{Root: root}, nil).processEnv("opencode", AgentConfig{})

	if env["OPENROUTER_API_KEY"] != "openrouter-key" || env["OPENAI_API_KEY"] != "" {
		t.Fatalf("OpenCode provider key isolation failed: %#v", env)
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

func TestProcessEnvWritesOpenCodeInstructionsWithSessionExtension(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(nil, Config{
		Root:         root,
		SystemPrompt: testPrompt("jaz instructions"),
	}, nil)
	env, err := manager.processEnvPreparedForSurface(context.Background(), "opencode", AgentConfig{}, "", "", []string{"run context"})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "acp", "opencode", "jaz-instructions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "jaz instructions\n\nrun context\n" {
		t.Fatalf("instructions = %q", data)
	}
	if !strings.Contains(env["OPENCODE_CONFIG_CONTENT"], path) {
		t.Fatalf("OPENCODE_CONFIG_CONTENT = %q", env["OPENCODE_CONFIG_CONTENT"])
	}
}

func TestProcessEnvWritesOpenCodeRestrictedWorkerInstructionsWithoutBasePrompt(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(nil, Config{
		Root:         root,
		SystemPrompt: testPrompt("jaz platform prompt"),
	}, nil)
	env, err := manager.processEnvPreparedForSurfacePolicy(context.Background(), "opencode", AgentConfig{}, "", "", MCPServerPolicyBrowserWorker, []string{"browser worker prompt"})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "acp", "opencode", "jaz-instructions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "browser worker prompt\n" {
		t.Fatalf("instructions = %q", data)
	}
	if !strings.Contains(env["OPENCODE_CONFIG_CONTENT"], path) {
		t.Fatalf("OPENCODE_CONFIG_CONTENT = %q", env["OPENCODE_CONFIG_CONTENT"])
	}
}

func TestProcessEnvWritesOpenCodeInstructionsWithResolvedCwd(t *testing.T) {
	root := t.TempDir()
	sessionCwd := filepath.Join(root, "workspaces", "default", ".worktrees", "loop-run")
	agentCwd := filepath.Join(root, "wrong")
	env, err := NewManager(nil, Config{
		Root:         root,
		SystemPrompt: cwdPrompt{},
	}, nil).processEnvPreparedForSurface(context.Background(), "opencode", AgentConfig{Cwd: agentCwd}, sessionCwd, "widget", nil)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "acp", "opencode", "jaz-instructions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "cwd="+sessionCwd+"\n" {
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

func TestProcessEnvAddsOpenCodeOpenRouterReasoningVariant(t *testing.T) {
	root := t.TempDir()
	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("opencode", AgentConfig{
		ProviderMode:    AgentProviderModeAgentDefaults,
		ModelProvider:   "openrouter",
		Model:           "z-ai/glm-5.2",
		ReasoningEffort: "xhigh",
	})
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		Provider map[string]struct {
			API    string `json:"api"`
			NPM    string `json:"npm"`
			Models map[string]struct {
				Variants map[string]struct {
					Reasoning struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
				} `json:"variants"`
			} `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatalf("config content = %q: %v", env["OPENCODE_CONFIG_CONTENT"], err)
	}
	openRouter := content.Provider["openrouter"]
	if openRouter.API != "" || openRouter.NPM != "" {
		t.Fatalf("openrouter provider should stay built-in: %#v", openRouter)
	}
	variant, ok := openRouter.Models["z-ai/glm-5.2"].Variants["xhigh"]
	if !ok || variant.Reasoning.Effort != "xhigh" {
		t.Fatalf("reasoning variant = %#v", openRouter.Models)
	}
}

func TestProcessEnvSetsOpenCodeSmallModelForOpenRouter(t *testing.T) {
	root := t.TempDir()
	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("opencode", AgentConfig{
		ProviderMode:  AgentProviderModeAgentDefaults,
		ModelProvider: "openrouter",
		Model:         "tencent/hy3:free",
	})
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		Model      string `json:"model"`
		SmallModel string `json:"small_model"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatalf("config content = %q: %v", env["OPENCODE_CONFIG_CONTENT"], err)
	}
	if content.Model != "openrouter/tencent/hy3:free" {
		t.Fatalf("model = %q", content.Model)
	}
	if content.SmallModel != "openrouter/tencent/hy3:free" {
		t.Fatalf("small_model = %q", content.SmallModel)
	}
}

func TestProcessEnvDoesNotAddOpenCodeReasoningVariantForInvalidEffort(t *testing.T) {
	root := t.TempDir()
	env, err := NewManager(nil, Config{Root: root}, nil).processEnvPrepared("opencode", AgentConfig{
		ProviderMode:    AgentProviderModeAgentDefaults,
		ModelProvider:   "openrouter",
		Model:           "z-ai/glm-5.2",
		ReasoningEffort: "ultracode",
	})
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		Provider map[string]struct {
			Models map[string]struct {
				Variants map[string]any `json:"variants"`
			} `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatalf("config content = %q: %v", env["OPENCODE_CONFIG_CONTENT"], err)
	}
	if variants := content.Provider["openrouter"].Models["z-ai/glm-5.2"].Variants; len(variants) != 0 {
		t.Fatalf("invalid effort created reasoning variants: %#v", variants)
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
			"unused": {
				Type:      "openai-compatible",
				BaseURL:   "https://unused.internal/v1",
				APIKey:    "unused-key",
				APIKeyEnv: "UNUSED_PROVIDER_KEY",
			},
		},
	}, nil).processEnvPrepared("opencode", AgentConfig{Model: "internal/chat"})
	if err != nil {
		t.Fatal(err)
	}
	if env["JAZ_PROVIDER_INTERNAL_API_KEY"] != "internal-key" {
		t.Fatalf("custom provider key not mapped: %#v", env)
	}
	if env["UNUSED_PROVIDER_KEY"] != "" {
		t.Fatalf("unselected provider key leaked into OpenCode: %#v", env)
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

func TestOpenCodeRejectsResponsesOnlyCustomProvider(t *testing.T) {
	providers := map[string]modelprovider.ModelProviderConfig{
		"internal": {
			Type:         "openai-compatible",
			BaseURL:      "https://llm.internal/v1",
			APIKey:       "internal-key",
			Capabilities: []string{modelprovider.CapabilityResponses},
		},
	}
	manager := NewManager(nil, Config{Providers: providers}, nil)
	if _, ok := manager.openCodeProviderConfig("internal/chat"); ok {
		t.Fatal("Responses-only provider yielded OpenCode config")
	}
	status := ProbeAgentAuthWithProviders(AgentOpenCode, AgentConfig{Model: "internal/chat"}, t.TempDir(), nil, providers)
	if status.Authenticated || !strings.Contains(status.Reason, "does not support Chat Completions") {
		t.Fatalf("auth = %#v", status)
	}
	env, err := manager.processEnvPrepared(AgentOpenCode, AgentConfig{Model: "internal/chat"})
	if err != nil {
		t.Fatal(err)
	}
	if env["JAZ_PROVIDER_INTERNAL_API_KEY"] != "" {
		t.Fatalf("incompatible provider key leaked into OpenCode: %#v", env)
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

func TestProbeAgentAuthRequiresAntigravityCLI(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()

	status := ProbeAgentAuth(AgentAntigravity, AgentConfig{}, root, nil)
	if status.Authenticated || !strings.Contains(status.Reason, "Antigravity CLI OAuth") {
		t.Fatalf("status = %#v, want missing antigravity CLI auth", status)
	}

	status = ProbeAgentAuth(AgentAntigravity, AgentConfig{}, root, map[string]string{
		"GEMINI_API_KEY": "managed-key",
	})
	if status.Authenticated || status.AuthKind == AuthKindAPIKey {
		t.Fatalf("status = %#v, GEMINI_API_KEY must not enable hidden Python SDK auth", status)
	}
}

func TestProbeAgentAuthDetectsAntigravityExistingCLI(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "agy"), "#!/bin/sh\nexit 0\n", "")
	t.Setenv("PATH", bin)

	status := ProbeAgentAuth(AgentAntigravity, AgentConfig{}, root, nil)
	if !status.Authenticated || status.AuthKind != AuthKindOAuth || status.AuthMode != AuthModeExistingCLI {
		t.Fatalf("status = %#v, want existing agy OAuth", status)
	}
}

func TestProcessEnvPreservesConfiguredAntigravityEnv(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	manager := NewManager(nil, Config{
		Root: root,
		Env:  map[string]string{"GEMINI_API_KEY": "managed-key"},
	}, nil)

	env := manager.processEnv(AgentAntigravity, AgentConfig{})
	if env["GEMINI_API_KEY"] != "managed-key" {
		t.Fatalf("GEMINI_API_KEY = %q, want managed key", env["GEMINI_API_KEY"])
	}
}

func TestProcessEnvAddsAntigravityLoginBinDirToPath(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	path := managedtool.ExecutablePath(root, managedtool.AntigravityCLI)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	env := NewManager(nil, Config{Root: root}, nil).processEnv(AgentAntigravity, AgentConfig{LoginBinDir: filepath.Dir(path)})
	if parts := filepath.SplitList(env["PATH"]); len(parts) == 0 || parts[0] != filepath.Dir(path) {
		t.Fatalf("PATH = %q, want managed agy dir first", env["PATH"])
	}
}

func TestProcessEnvPrefersAccountAuthOverExplicitAPIKeys(t *testing.T) {
	root := t.TempDir()
	codexHome := t.TempDir()
	writeCodexOAuth(t, codexHome)
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
	writeCodexOAuth(t, codexHome)
	ready = ProbeReadiness(AgentCodex, AgentConfig{Command: exe}, t.TempDir(), map[string]string{"CODEX_HOME": codexHome})
	if !ready.Available {
		t.Fatalf("codex should be ready with oauth auth: %#v", ready)
	}
}

func TestProbeReadinessRequiresAntigravityCLIOAuth(t *testing.T) {
	clearHostEnv(t)
	exe := testExecutable(t)

	ready := ProbeReadiness(AgentAntigravity, AgentConfig{Command: exe}, t.TempDir(), nil)
	if ready.Available || !strings.Contains(ready.Reason, "Antigravity CLI OAuth") {
		t.Fatalf("ready = %#v, want missing antigravity CLI OAuth", ready)
	}

	ready = ProbeReadiness(AgentAntigravity, AgentConfig{Command: exe}, t.TempDir(), map[string]string{"GEMINI_API_KEY": "key"})
	if ready.Available {
		t.Fatalf("GEMINI_API_KEY must not make Antigravity ready through Python SDK fallback: %#v", ready)
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

func TestProbeReadinessRejectsURLBackedGrokModelOverride(t *testing.T) {
	ready := ProbeReadiness(AgentGrok, AgentConfig{URL: "http://127.0.0.1:9999", Model: modelcatalog.DefaultGrokModel}, t.TempDir(), nil)
	if ready.Available || !strings.Contains(ready.Reason, "URL-backed Grok") {
		t.Fatalf("ready = %#v", ready)
	}
}

func TestProbeReadinessRejectsURLBackedQwen(t *testing.T) {
	ready := ProbeReadiness(AgentQwen, AgentConfig{URL: "http://127.0.0.1:9999"}, t.TempDir(), nil)
	if ready.Available || !strings.Contains(ready.Reason, "model and Jaz system prompt require the local agent launch") {
		t.Fatalf("readiness = %#v", ready)
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

func TestAutoAuthMethodPrefersCodexAPIKeyOverStaleOAuth(t *testing.T) {
	codexHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	method, missing := autoAuthMethod("codex", codexInitializeAuthMethods(), map[string]string{
		"CODEX_HOME":     codexHome,
		"OPENAI_API_KEY": "key",
	})

	if method != "openai-api-key" || len(missing) != 0 {
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

	method, missing = autoAuthMethod("grok", grokInitializeEnvAuthMethods(), map[string]string{
		"HOME":        home,
		"XAI_API_KEY": "key",
	})
	if method != "cached_token" || len(missing) != 0 {
		t.Fatalf("env auth method=%q missing=%v", method, missing)
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
	_, args, err := processCommand("grok", AgentConfig{
		Command:         "grok",
		Args:            []string{"--no-auto-update", "agent", "--no-leader", "stdio"},
		ReasoningEffort: "high",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "--no-auto-update agent --no-leader --always-approve --reasoning-effort high stdio"
	if strings.Join(args, " ") != want {
		t.Fatalf("args = %q, want %q", strings.Join(args, " "), want)
	}
}

func TestProcessCommandAddsGrokModelArg(t *testing.T) {
	_, args, err := processCommand("grok", AgentConfig{
		Command: "grok",
		Args:    []string{"agent", "stdio"},
		Model:   modelcatalog.DefaultGrokModel,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "agent --always-approve --model grok-4.5 stdio" {
		t.Fatalf("args = %#v", args)
	}
}

func TestProcessCommandRejectsAmbiguousGrokModelArg(t *testing.T) {
	_, _, err := processCommand("grok", AgentConfig{
		Command: "grok",
		Args:    []string{"agent", "--model=custom", "stdio"},
		Model:   modelcatalog.DefaultGrokModel,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "model is ambiguous") {
		t.Fatalf("error = %v", err)
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
			_, args, err := processCommand("grok", AgentConfig{
				Command: "grok",
				Args:    tc.args,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(args, " ") != tc.want {
				t.Fatalf("args = %q, want %q", strings.Join(args, " "), tc.want)
			}
		})
	}
}

func TestProcessCommandRejectsAmbiguousGrokReasoningEffortArg(t *testing.T) {
	_, _, err := processCommand("grok", AgentConfig{
		Command:         "grok",
		Args:            []string{"agent", "--reasoning-effort=low", "stdio"},
		ReasoningEffort: "high",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "reasoning effort is ambiguous") {
		t.Fatalf("error = %v", err)
	}
}

func TestProcessCommandLeavesNonGrokCommandAlone(t *testing.T) {
	_, args, err := processCommand("grok", AgentConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestFakeACPAgentProcess"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
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

func grokInitializeEnvAuthMethods() []byte {
	return []byte(`{
		"authMethods": [
			{"id": "cached_token", "name": "cached_token"},
			{"type": "env_var", "id": "xai.api_key", "vars": [{"name": "XAI_API_KEY"}]}
		]
	}`)
}

func testExecutable(t *testing.T) string {
	t.Helper()
	return testexec.Write(t, filepath.Join(t.TempDir(), "agent"), "", "")
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
