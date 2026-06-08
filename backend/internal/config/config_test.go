package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	openrouterprovider "github.com/wins/jaz/backend/internal/provider/openrouter"
)

func TestApplyProviderSelectsOpenAI(t *testing.T) {
	cfg := Config{
		Providers:  ProvidersConfig{Default: "openai"},
		OpenRouter: openrouterprovider.Config{APIKey: "openrouter-key", Model: "openai/gpt-5.4-mini", ReasoningEffort: "high"},
		OpenAI:     openaiprovider.Config{APIKey: "openai-key", Model: "gpt-4.1-mini", ReasoningEffort: "low"},
	}

	if err := applyProvider(&cfg); err != nil {
		t.Fatal(err)
	}

	provider := cfg.Jaz.Provider
	if provider.Type != "openai" || provider.APIKey != "openai-key" || provider.Model != "gpt-4.1-mini" || provider.ReasoningEffort != "low" {
		t.Fatalf("unexpected provider %#v", provider)
	}
}

func TestApplyProviderSelectsOpenRouter(t *testing.T) {
	cfg := Config{
		Providers:  ProvidersConfig{Default: "openrouter"},
		OpenRouter: openrouterprovider.Config{APIKey: "openrouter-key", Model: "openai/gpt-5.4-mini", ReasoningEffort: "high"},
		OpenAI:     openaiprovider.Config{APIKey: "openai-key", Model: "gpt-4.1-mini"},
	}

	if err := applyProvider(&cfg); err != nil {
		t.Fatal(err)
	}

	provider := cfg.Jaz.Provider
	if provider.Type != "openrouter" || provider.APIKey != "openrouter-key" || provider.Model != "openai/gpt-5.4-mini" || provider.ReasoningEffort != "high" {
		t.Fatalf("unexpected provider %#v", provider)
	}
}

func TestProviderKeysStayOutOfACPEnv(t *testing.T) {
	cfg := Config{
		Providers: ProvidersConfig{Default: "openai"},
		OpenAI:    openaiprovider.Config{APIKey: "openai-key", Model: "gpt-4.1-mini"},
	}

	if err := applyProvider(&cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Jaz.ACP.Env["OPENAI_API_KEY"] != "" {
		t.Fatalf("provider API key leaked into acp env: %#v", cfg.Jaz.ACP.Env)
	}
}

func TestLoadConfigUnmarshalsACPAgentModel(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	path := filepath.Join(t.TempDir(), "application.yaml")
	if err := os.WriteFile(path, []byte(`
jaz:
  acp:
    agents:
      codex:
        command: codex-acp
        model: gpt-5.5
        reasoningeffort: high
providers:
  default: openrouter
openrouter:
  apikey: openrouter-key
  model: openai/gpt-5.4-mini
  reasoningeffort: medium
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPLICATION_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	agent, ok := cfg.Jaz.ACP.Agent("codex")
	if !ok {
		t.Fatal("codex agent config missing")
	}
	if agent.Model != "gpt-5.5" {
		t.Fatalf("agent model = %q", agent.Model)
	}
	if agent.ReasoningEffort != "high" {
		t.Fatalf("agent reasoning effort = %q", agent.ReasoningEffort)
	}
	if cfg.Jaz.Provider.Type != "openrouter" || cfg.Jaz.Provider.Model != "openai/gpt-5.4-mini" || cfg.Jaz.Provider.ReasoningEffort != "medium" {
		t.Fatalf("unexpected native provider config %#v", cfg.Jaz.Provider)
	}
}
