package config

import (
	"testing"

	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	openrouterprovider "github.com/wins/jaz/backend/internal/provider/openrouter"
)

func TestApplyProviderSelectsOpenAI(t *testing.T) {
	cfg := Config{
		Providers:  ProvidersConfig{Default: "openai"},
		OpenRouter: openrouterprovider.Config{APIKey: "openrouter-key", Model: "openai/gpt-5.4-mini"},
		OpenAI:     openaiprovider.Config{APIKey: "openai-key", Model: "gpt-4.1-mini"},
	}

	if err := applyProvider(&cfg); err != nil {
		t.Fatal(err)
	}

	provider := cfg.Jaz.Provider
	if provider.Type != "openai" || provider.APIKey != "openai-key" || provider.Model != "gpt-4.1-mini" {
		t.Fatalf("unexpected provider %#v", provider)
	}
}

func TestApplyProviderSelectsOpenRouter(t *testing.T) {
	cfg := Config{
		Providers:  ProvidersConfig{Default: "openrouter"},
		OpenRouter: openrouterprovider.Config{APIKey: "openrouter-key", Model: "openai/gpt-5.4-mini"},
		OpenAI:     openaiprovider.Config{APIKey: "openai-key", Model: "gpt-4.1-mini"},
	}

	if err := applyProvider(&cfg); err != nil {
		t.Fatal(err)
	}

	provider := cfg.Jaz.Provider
	if provider.Type != "openrouter" || provider.APIKey != "openrouter-key" || provider.Model != "openai/gpt-5.4-mini" {
		t.Fatalf("unexpected provider %#v", provider)
	}
}
