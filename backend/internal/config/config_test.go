package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/provider"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	openrouterprovider "github.com/wins/jaz/backend/internal/provider/openrouter"
)

func TestApplyProviderBuildsModelProviderCatalog(t *testing.T) {
	cfg := Config{
		OpenRouter: openrouterprovider.Config{APIKey: "openrouter-key"},
		OpenAI:     openaiprovider.Config{APIKey: "openai-key"},
		Jaz: appConfigWithProvider("internal", provider.ModelProviderConfig{
			Type:         "openai-compatible",
			Label:        "Internal",
			BaseURL:      "https://llm.internal/v1",
			APIKeyEnv:    "INTERNAL_LLM_API_KEY",
			Capabilities: []string{provider.CapabilityChatCompletions},
		}),
	}

	if err := applyProvider(&cfg); err != nil {
		t.Fatal(err)
	}

	openRouter := cfg.Jaz.ModelProviders["openrouter"]
	if openRouter.Type != "openrouter" || openRouter.APIKey != "openrouter-key" || openRouter.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("unexpected openrouter catalog entry %#v", openRouter)
	}
	openAI := cfg.Jaz.ModelProviders["openai"]
	if openAI.Type != "openai" || openAI.APIKey != "openai-key" || openAI.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected openai catalog entry %#v", openAI)
	}
	if _, ok := cfg.Jaz.ModelProviders["anthropic"]; ok {
		t.Fatalf("anthropic should not be a runnable model provider: %#v", cfg.Jaz.ModelProviders)
	}
	internal := cfg.Jaz.ModelProviders["internal"]
	if internal.Type != "openai-compatible" || internal.BaseURL != "https://llm.internal/v1" ||
		len(internal.Capabilities) != 1 || internal.Capabilities[0] != provider.CapabilityChatCompletions {
		t.Fatalf("custom provider not preserved: %#v", internal)
	}
}

func appConfigWithProvider(id string, cfg provider.ModelProviderConfig) app.Config {
	return app.Config{ModelProviders: map[string]provider.ModelProviderConfig{id: cfg}}
}

func TestProviderKeysStayOutOfACPEnv(t *testing.T) {
	cfg := Config{
		OpenAI: openaiprovider.Config{APIKey: "openai-key"},
	}

	if err := applyProvider(&cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Jaz.ACP.Env["OPENAI_API_KEY"] != "" {
		t.Fatalf("provider API key leaked into acp env: %#v", cfg.Jaz.ACP.Env)
	}
}

func TestLoadConfigUnmarshalsACPAgentModelAndProviderKeys(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	path := filepath.Join(t.TempDir(), "application.yaml")
	if err := os.WriteFile(path, []byte(`
jaz:
  connections:
    chat:
      grouphistorylimit: 123
  acp:
    agents:
      codex:
        command: codex-acp
        model: gpt-5.5
        reasoningeffort: high
openrouter:
  apikey: openrouter-key
openai:
  apikey: openai-key
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPLICATION_CONFIG", path)
	t.Setenv("OPENROUTER_API_KEY", "openrouter-key")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("JAZ_SKILLS_DISABLE_SYNC", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	agent, ok := cfg.Jaz.ACP.Agents["codex"]
	if !ok {
		t.Fatal("codex agent config missing")
	}
	if agent.Model != "gpt-5.5" {
		t.Fatalf("agent model = %q", agent.Model)
	}
	if agent.ReasoningEffort != "high" {
		t.Fatalf("agent reasoning effort = %q", agent.ReasoningEffort)
	}
	if cfg.Jaz.ModelProviders["openrouter"].APIKey != "openrouter-key" || cfg.Jaz.ModelProviders["openai"].APIKey != "openai-key" {
		t.Fatalf("unexpected model provider catalog %#v", cfg.Jaz.ModelProviders)
	}
	if !cfg.Jaz.Skills.DisableSync {
		t.Fatalf("skills config = %#v", cfg.Jaz.Skills)
	}
	if cfg.Jaz.Connections.Chat.GroupHistoryLimit != 123 {
		t.Fatalf("group history limit = %d", cfg.Jaz.Connections.Chat.GroupHistoryLimit)
	}
}
