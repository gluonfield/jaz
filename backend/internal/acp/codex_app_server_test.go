package acp

import (
	"encoding/json"
	"reflect"
	"testing"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

func TestConfigureCodexAppServerEnv(t *testing.T) {
	env := map[string]string{
		"CODEX_CONFIG":        `{"features":{"existing":true},"model_providers":{"keep":{"name":"Keep"}}}`,
		codexModelMetadataEnv: `{"id":"moonshotai/kimi-k3","display_name":"Kimi K3","context_window":1048576}`,
	}
	cfg := AgentConfig{
		ModelProvider:   modelprovider.ProviderOpenRouter,
		Model:           "moonshotai/kimi-k3",
		ReasoningEffort: "max",
	}
	providers := map[string]modelprovider.ModelProviderConfig{
		modelprovider.ProviderOpenRouter: {
			Label:        "OpenRouter",
			BaseURL:      "https://openrouter.ai/api/v1",
			APIKeyEnv:    "OPENROUTER_API_KEY",
			Capabilities: []string{modelprovider.CapabilityResponses},
		},
	}

	if err := configureCodexAppServerEnv(env, cfg, providers, "Jaz instructions"); err != nil {
		t.Fatal(err)
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(env["CODEX_CONFIG"]), &config); err != nil {
		t.Fatal(err)
	}
	if env["MODEL_PROVIDER"] != modelprovider.ProviderOpenRouter {
		t.Fatalf("MODEL_PROVIDER = %q", env["MODEL_PROVIDER"])
	}
	if config["developer_instructions"] != "Jaz instructions" ||
		config["model"] != "moonshotai/kimi-k3" ||
		config["model_reasoning_effort"] != "max" ||
		config["model_context_window"] != float64(1_048_576) ||
		config["sandbox_mode"] != "danger-full-access" ||
		config["approval_policy"] != "never" {
		t.Fatalf("config = %#v", config)
	}
	features := config["features"].(map[string]any)
	if features["existing"] != true || features["goals"] != false ||
		features["tool_search_always_defer_mcp_tools"] != true {
		t.Fatalf("features = %#v", features)
	}
	modelProviders := config["model_providers"].(map[string]any)
	if !reflect.DeepEqual(modelProviders["keep"], map[string]any{"name": "Keep"}) {
		t.Fatalf("preserved providers = %#v", modelProviders)
	}
	if !reflect.DeepEqual(modelProviders[modelprovider.ProviderOpenRouter], map[string]any{
		"name":     "OpenRouter",
		"base_url": "https://openrouter.ai/api/v1",
		"env_key":  "OPENROUTER_API_KEY",
		"wire_api": "responses",
	}) {
		t.Fatalf("OpenRouter = %#v", modelProviders[modelprovider.ProviderOpenRouter])
	}
}

func TestConfigureCodexAppServerEnvKeepsOpenAIAccountAuthNative(t *testing.T) {
	env := map[string]string{"CODEX_CONFIG": "null"}
	if err := configureCodexAppServerEnv(env, AgentConfig{
		ModelProvider: modelprovider.ProviderOpenAI,
		Model:         modelprovider.OpenAIModelGPT56Sol,
	}, nil, "instructions"); err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(env["CODEX_CONFIG"]), &config); err != nil {
		t.Fatal(err)
	}
	if env["MODEL_PROVIDER"] != "openai" || config["model_provider"] != "openai" {
		t.Fatalf("env = %#v, config = %#v", env, config)
	}
	if _, ok := config["model_providers"]; ok {
		t.Fatalf("native OpenAI provider was overridden: %#v", config)
	}
}

func TestCodexAppServerOwnsLaunchPrompt(t *testing.T) {
	cfg := AgentConfig{ManagedAdapter: codexAppServerAdapter}
	if !systemPromptAtLaunch(AgentCodex, cfg) {
		t.Fatal("official Codex ACP must receive developer instructions at launch")
	}
	if systemPromptAtLaunch(AgentCodex, AgentConfig{ManagedAdapter: "codex"}) {
		t.Fatal("legacy Codex ACP must keep its session metadata prompt path")
	}
}
