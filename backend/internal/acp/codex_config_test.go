package acp

import (
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

func TestCodexProviderArgsOpenAIAPIKey(t *testing.T) {
	args := codexProviderArgs(AgentConfig{ModelProvider: CodexProviderOpenAIAPIKey}, nil)
	want := []string{
		"-c", `model_provider="openai-api-key"`,
		"-c", `model_providers.openai-api-key.name="OpenAI"`,
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

func TestCodexProviderArgsUnknownWithoutConfig(t *testing.T) {
	if args := codexProviderArgs(AgentConfig{ModelProvider: "ghost"}, nil); args != nil {
		t.Fatalf("unknown provider with no base_url/env_key should yield no args, got %v", args)
	}
}

func TestProcessEnvBindsSelectedCodexProviderKeyOnly(t *testing.T) {
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
	if openrouter["OPENROUTER_API_KEY"] != "or-key" {
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
