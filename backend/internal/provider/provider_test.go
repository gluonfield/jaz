package provider

import "testing"

func TestMessageContentExtractsTextFromMultipartUserMessage(t *testing.T) {
	msg := UserMessageParts(
		TextPart("look at this"),
		ImageURLPart("data:image/png;base64,abc", "auto"),
	)
	if got := MessageContent(msg); got != "look at this" {
		t.Fatalf("MessageContent() = %q, want text part", got)
	}
}

func TestApplyModelProviderConfigKeepsOllamaNoKey(t *testing.T) {
	meta, ok := ModelProviderByID(ProviderOllama)
	if !ok {
		t.Fatal("ollama provider missing")
	}
	got := ApplyModelProviderConfig(meta, ModelProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "http://localhost:11434/v1",
	})
	if got.RequiresAPIKey || got.APIKeyEnv != "" {
		t.Fatalf("ollama key metadata = requires %v env %q", got.RequiresAPIKey, got.APIKeyEnv)
	}
}

func TestApplyModelProviderConfigGeneratesKeyEnvForCustomProvider(t *testing.T) {
	got := ApplyModelProviderConfig(ModelProvider{ID: "internal"}, ModelProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://llm.internal/v1",
	})
	if !got.RequiresAPIKey || got.APIKeyEnv != "JAZ_PROVIDER_INTERNAL_API_KEY" {
		t.Fatalf("custom provider key metadata = requires %v env %q", got.RequiresAPIKey, got.APIKeyEnv)
	}
}

func TestApplyModelProviderConfigKeepsLoopbackCustomNoKey(t *testing.T) {
	got := ApplyModelProviderConfig(ModelProvider{ID: "local"}, ModelProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "http://127.0.0.1:11434/v1",
	})
	if got.RequiresAPIKey || got.APIKeyEnv != "" {
		t.Fatalf("loopback provider key metadata = requires %v env %q", got.RequiresAPIKey, got.APIKeyEnv)
	}
}

func TestApplyModelProviderConfigAllowsExplicitKeyForNoKeyBuiltin(t *testing.T) {
	meta, ok := ModelProviderByID(ProviderOllama)
	if !ok {
		t.Fatal("ollama provider missing")
	}
	got := ApplyModelProviderConfig(meta, ModelProviderConfig{APIKey: "ollama-key"})
	if !got.RequiresAPIKey || got.APIKeyEnv != "JAZ_PROVIDER_OLLAMA_API_KEY" {
		t.Fatalf("ollama explicit key metadata = requires %v env %q", got.RequiresAPIKey, got.APIKeyEnv)
	}
}

func TestCustomOpenAICompatibleProviderDefaultsToChatCompletions(t *testing.T) {
	custom := ApplyModelProviderConfig(ModelProvider{ID: "custom"}, ModelProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1",
	})
	if !custom.SupportsCapability(CapabilityChatCompletions) || custom.SupportsCapability(CapabilityResponses) {
		t.Fatalf("custom OpenAI-compatible provider must default to Chat Completions only: %#v", custom)
	}
	invalid := ApplyModelProviderConfig(ModelProvider{ID: "invalid"}, ModelProviderConfig{
		Type:         "openai-compatible",
		BaseURL:      "https://invalid.example/v1",
		Capabilities: []string{"response"},
	})
	if len(invalid.Capabilities) != 0 {
		t.Fatalf("invalid explicit capabilities must fail closed: %#v", invalid)
	}
	both := ApplyModelProviderConfig(ModelProvider{ID: "both"}, ModelProviderConfig{
		Type:         "openai-compatible",
		BaseURL:      "https://both.example/v1",
		Capabilities: []string{CapabilityResponses, CapabilityChatCompletions},
	})
	if !both.SupportsCapability(CapabilityChatCompletions) || !both.SupportsCapability(CapabilityResponses) {
		t.Fatalf("explicit provider capabilities lost: %#v", both)
	}
}

func TestResolveModelProvidersAppliesConfiguredDefaultsOnce(t *testing.T) {
	resolved := ResolveModelProviders(map[string]ModelProviderConfig{
		ProviderOpenRouter: {DefaultModel: "router-custom"},
		"acme": {
			Type:         "openai-compatible",
			BaseURL:      "https://acme.test/v1",
			DefaultModel: "acme-coder",
		},
	})
	seen := map[string]ResolvedModelProvider{}
	for _, modelProvider := range resolved {
		if _, duplicate := seen[modelProvider.ID]; duplicate {
			t.Fatalf("duplicate provider %q in %#v", modelProvider.ID, resolved)
		}
		seen[modelProvider.ID] = modelProvider
	}
	if seen[ProviderOpenRouter].Meta.DefaultModel != "router-custom" || seen["acme"].Meta.DefaultModel != "acme-coder" {
		t.Fatalf("resolved providers = %#v", seen)
	}
}
