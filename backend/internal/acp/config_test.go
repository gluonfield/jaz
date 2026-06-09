package acp

import (
	"os"
	"path/filepath"
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
	if env["HOME"] == home {
		t.Fatal("subprocess HOME should stay isolated")
	}
}

func TestProcessEnvUsesUserHomeForClaudeCode(t *testing.T) {
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

	if env["HOME"] != home {
		t.Fatalf("HOME = %q, want %q", env["HOME"], home)
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
	if env["CLAUDE_CONFIG_DIR"] != configDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", env["CLAUDE_CONFIG_DIR"], configDir)
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
