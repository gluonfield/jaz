package acp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
)

func TestCodexProviderArgsNativeOpenAI(t *testing.T) {
	for _, providerID := range []string{"", AgentCodex, modelprovider.ProviderOpenAI} {
		if args := codexProviderArgs(AgentConfig{ModelProvider: providerID}, nil); args != nil {
			t.Fatalf("provider %q should use the native Codex path, got %v", providerID, args)
		}
	}
}

func TestCodexProviderArgsOpenRouter(t *testing.T) {
	args := codexProviderArgs(AgentConfig{ModelProvider: modelprovider.ProviderOpenRouter}, nil)
	want := []string{
		"-c", `model_provider="openrouter"`,
		"-c", `model_providers.openrouter.name="OpenRouter"`,
		"-c", `model_providers.openrouter.base_url="https://openrouter.ai/api/v1"`,
		"-c", `model_providers.openrouter.env_key="OPENROUTER_API_KEY"`,
		"-c", `model_providers.openrouter.wire_api="responses"`,
	}
	if !slices.Equal(args, want) {
		t.Fatalf("openrouter args mismatch\n got: %v\nwant: %v", args, want)
	}
}

func TestCodexProviderArgsModelStudioUsesResponses(t *testing.T) {
	args := codexProviderArgs(AgentConfig{ModelProvider: modelprovider.ProviderModelStudio}, nil)
	want := []string{
		"-c", `model_provider="modelstudio-us"`,
		"-c", `model_providers.modelstudio-us.name="Alibaba ModelStudio (US)"`,
		"-c", `model_providers.modelstudio-us.base_url="https://dashscope-us.aliyuncs.com/compatible-mode/v1"`,
		"-c", `model_providers.modelstudio-us.env_key="DASHSCOPE_API_KEY"`,
		"-c", `model_providers.modelstudio-us.wire_api="responses"`,
	}
	if !slices.Equal(args, want) {
		t.Fatalf("ModelStudio args mismatch\n got: %v\nwant: %v", args, want)
	}
}

func TestCodexProviderArgsOllama(t *testing.T) {
	want := []string{"-c", `model_provider="ollama"`}
	if args := codexProviderArgs(AgentConfig{ModelProvider: modelprovider.ProviderOllama}, nil); !slices.Equal(args, want) {
		t.Fatalf("ollama args mismatch\n got: %v\nwant: %v", args, want)
	}
}

func TestProbeAgentAuthAcceptsNoAuthCodexProvider(t *testing.T) {
	status := ProbeAgentAuthWithProviders(AgentCodex, AgentConfig{
		ProviderMode:  AgentProviderModeAgentDefaults,
		ModelProvider: modelprovider.ProviderOllama,
	}, t.TempDir(), nil, nil)
	if !status.Authenticated || status.AuthKind != AuthKindNone || status.AuthEvidence != "no_api_key_required" {
		t.Fatalf("ollama auth = %#v", status)
	}
}

func TestCodexProviderArgsOpenAIAPIKey(t *testing.T) {
	args := codexProviderArgs(AgentConfig{ModelProvider: CodexProviderOpenAIAPIKey}, nil)
	want := []string{
		"-c", `model_provider="openai-api-key"`,
		"-c", `model_providers.openai-api-key.name="OpenAI API key"`,
		"-c", `model_providers.openai-api-key.base_url="https://api.openai.com/v1"`,
		"-c", `model_providers.openai-api-key.env_key="OPENAI_API_KEY"`,
		"-c", `model_providers.openai-api-key.wire_api="responses"`,
	}
	if !slices.Equal(args, want) {
		t.Fatalf("openai api-key args mismatch\n got: %v\nwant: %v", args, want)
	}
}

func TestCodexProviderArgsCustomProvider(t *testing.T) {
	args := codexProviderArgs(
		AgentConfig{ModelProvider: "acme"},
		map[string]modelprovider.ModelProviderConfig{
			"acme": {Type: "openai-compatible", Label: "Acme", BaseURL: "https://acme.test/v1", APIKeyEnv: "ACME_KEY"},
		},
	)
	want := []string{
		"-c", `model_provider="acme"`,
		"-c", `model_providers.acme.name="Acme"`,
		"-c", `model_providers.acme.base_url="https://acme.test/v1"`,
		"-c", `model_providers.acme.env_key="ACME_KEY"`,
		"-c", `model_providers.acme.wire_api="responses"`,
	}
	if !slices.Equal(args, want) {
		t.Fatalf("custom provider args mismatch\n got: %v\nwant: %v", args, want)
	}
}

func TestCodexProviderArgsNoAuthCustomProvider(t *testing.T) {
	args := codexProviderArgs(
		AgentConfig{ModelProvider: "local"},
		map[string]modelprovider.ModelProviderConfig{
			"local": {Type: "openai-compatible", Label: "Local", BaseURL: "http://localhost:11434/v1"},
		},
	)
	want := []string{
		"-c", `model_provider="local"`,
		"-c", `model_providers.local.name="Local"`,
		"-c", `model_providers.local.base_url="http://localhost:11434/v1"`,
		"-c", `model_providers.local.wire_api="responses"`,
	}
	if !slices.Equal(args, want) {
		t.Fatalf("local provider args mismatch\n got: %v\nwant: %v", args, want)
	}
}

func TestCodexProviderArgsUnknownWithoutConfig(t *testing.T) {
	if args := codexProviderArgs(AgentConfig{ModelProvider: "ghost"}, nil); args != nil {
		t.Fatalf("unknown provider with no base_url/env_key should yield no args, got %v", args)
	}
}

func TestProcessEnvBindsSelectedCodexProviderKey(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	t.Setenv("PATH", "/bin")
	t.Setenv("HOME", t.TempDir())
	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{
		"OPENAI_API_KEY":     "oa-key",
		"OPENROUTER_API_KEY": "or-key",
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(nil, Config{Root: root}, nil)

	openrouter := manager.processEnv("codex", AgentConfig{ModelProvider: modelprovider.ProviderOpenRouter})
	if openrouter["OPENROUTER_API_KEY"] != "or-key" || openrouter["OPENAI_API_KEY"] != "" {
		t.Fatalf("codex+openrouter did not bind the provider key: %#v", openrouter)
	}

	openaiKey := manager.processEnv("codex", AgentConfig{ModelProvider: CodexProviderOpenAIAPIKey})
	if openaiKey["OPENAI_API_KEY"] != "oa-key" || openaiKey["OPENROUTER_API_KEY"] != "" {
		t.Fatalf("codex+openai api-key bound wrong provider keys: %#v", openaiKey)
	}

	openai := manager.processEnv("codex", AgentConfig{ModelProvider: modelprovider.ProviderOpenAI})
	if openai["OPENAI_API_KEY"] != "" || openai["OPENROUTER_API_KEY"] != "" {
		t.Fatalf("codex default (OAuth) must not receive provider API keys: %#v", openai)
	}
}

func TestCodexProviderKeyCannotOverwriteOAuthProfile(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	home := filepath.Join(root, "acp", "codex-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	credential := filepath.Join(home, "auth.json")
	original := `{"auth_mode":"chatgpt","tokens":{"access_token":"oauth","refresh_token":"refresh"}}`
	if err := os.WriteFile(credential, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(nil, Config{
		Root: root,
		Providers: map[string]modelprovider.ModelProviderConfig{
			modelprovider.ProviderOpenRouter: {APIKey: "or-key"},
		},
	}, nil)
	status := ProbeAgentAuthWithProviders(AgentCodex, AgentConfig{ModelProvider: modelprovider.ProviderOpenRouter}, root, nil, manager.providers())
	if status.StoragePath != "" {
		t.Fatalf("provider auth owns OAuth storage %q", status.StoragePath)
	}

	env := manager.processEnv(AgentCodex, AgentConfig{ModelProvider: modelprovider.ProviderOpenRouter})
	if env["CODEX_HOME"] != home {
		t.Fatalf("CODEX_HOME = %q, want stable session home %q", env["CODEX_HOME"], home)
	}
	data, err := os.ReadFile(credential)
	if err != nil || string(data) != original {
		t.Fatalf("provider setup changed Codex OAuth: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("provider setup wrote shared Codex config: %v", err)
	}
}

func TestProcessEnvBindsCodexCustomProviderKey(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	t.Setenv("PATH", "/bin")
	t.Setenv("HOME", t.TempDir())
	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{
		"ACME_KEY": "acme-key",
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(nil, Config{
		Root: root,
		Providers: map[string]modelprovider.ModelProviderConfig{
			"acme": {Type: "openai-compatible", BaseURL: "https://acme.test/v1", APIKeyEnv: "ACME_KEY"},
		},
	}, nil)

	env := manager.processEnv("codex", AgentConfig{ModelProvider: "acme"})
	if env["ACME_KEY"] != "acme-key" {
		t.Fatalf("custom provider key was not bound to its configured env: %#v", env)
	}
	if env["OPENAI_API_KEY"] != "" {
		t.Fatalf("custom provider key leaked into OPENAI_API_KEY: %#v", env)
	}
}
