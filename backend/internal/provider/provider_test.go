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

func TestQwenProviderCompatibilityMatchesWireSupport(t *testing.T) {
	modelStudio, ok := ModelProviderByID(ProviderModelStudio)
	if !ok || !modelStudio.SupportsCapability(CapabilityOpenCode) || !modelStudio.SupportsCapability(CapabilityCodex) {
		t.Fatalf("ModelStudio capabilities = %#v", modelStudio)
	}
	codingPlan, ok := ModelProviderByID(ProviderQwenCodingPlan)
	if !ok || !codingPlan.SupportsCapability(CapabilityOpenCode) || codingPlan.SupportsCapability(CapabilityCodex) {
		t.Fatalf("Coding Plan capabilities = %#v", codingPlan)
	}
	custom := ApplyModelProviderConfig(ModelProvider{ID: "custom"}, ModelProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://qwen.example/v1",
	})
	if !custom.SupportsCapability(CapabilityOpenCode) {
		t.Fatalf("custom OpenAI-compatible provider must support Qwen: %#v", custom)
	}
}
