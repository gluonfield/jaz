package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
)

func TestConfigureCodexEnv(t *testing.T) {
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

	if err := configureCodexEnv(env, cfg, providers, "Jaz instructions"); err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(env["CODEX_CONFIG"]), &config); err != nil {
		t.Fatal(err)
	}
	if config["developer_instructions"] != "Jaz instructions" ||
		config["model"] != "moonshotai/kimi-k3" ||
		config["model_reasoning_effort"] != "max" ||
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

func TestConfigureCodexEnvKeepsOpenAIAccountAuthNative(t *testing.T) {
	env := map[string]string{"CODEX_CONFIG": "null"}
	if err := configureCodexEnv(env, AgentConfig{
		ModelProvider: modelprovider.ProviderOpenAI,
		Model:         modelprovider.OpenAIModelGPT56Sol,
	}, nil, "instructions"); err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(env["CODEX_CONFIG"]), &config); err != nil {
		t.Fatal(err)
	}
	if config["model_provider"] != "openai" {
		t.Fatalf("env = %#v, config = %#v", env, config)
	}
	if _, ok := config["model_providers"]; ok {
		t.Fatalf("native OpenAI provider was overridden: %#v", config)
	}
}

func TestCodexLaunchProvider(t *testing.T) {
	providers := map[string]modelprovider.ModelProviderConfig{
		"acme": {
			Label:        "Acme",
			BaseURL:      "https://acme.test/v1",
			APIKeyEnv:    "ACME_KEY",
			Capabilities: []string{modelprovider.CapabilityResponses},
		},
	}
	for _, test := range []struct {
		id         string
		wantID     string
		wantConfig map[string]any
	}{
		{modelprovider.ProviderOpenAI, "openai", nil},
		{modelprovider.ProviderOllama, modelprovider.ProviderOllama, nil},
		{CodexProviderOpenAIAPIKey, CodexProviderOpenAIAPIKey, map[string]any{
			"name": "OpenAI API key", "base_url": "https://api.openai.com/v1",
			"env_key": "OPENAI_API_KEY", "wire_api": "responses",
		}},
		{"acme", "acme", map[string]any{
			"name": "Acme", "base_url": "https://acme.test/v1",
			"env_key": "ACME_KEY", "wire_api": "responses",
		}},
	} {
		id, config := codexLaunchProvider(AgentConfig{ModelProvider: test.id}, providers)
		if id != test.wantID || !reflect.DeepEqual(config, test.wantConfig) {
			t.Errorf("%s = %q, %#v; want %q, %#v", test.id, id, config, test.wantID, test.wantConfig)
		}
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

func TestCodexOpenAIAPIKeyRequiresResponsesCapability(t *testing.T) {
	providers := map[string]modelprovider.ModelProviderConfig{
		modelprovider.ProviderOpenAI: {Capabilities: []string{modelprovider.CapabilityChatCompletions}},
	}
	if _, ok := codexProvider(CodexProviderOpenAIAPIKey, providers); ok {
		t.Fatal("Chat-only OpenAI override yielded a Codex Responses provider")
	}
	resolved := modelprovider.ResolveModelProviders(providers)
	metas := make([]modelprovider.ModelProvider, 0, len(resolved))
	for _, modelProvider := range resolved {
		metas = append(metas, modelProvider.Meta)
	}
	ids := codexModelProviderIDs(metas)
	if !slices.Contains(ids, modelprovider.ProviderOpenAI) || slices.Contains(ids, CodexProviderOpenAIAPIKey) {
		t.Fatalf("Codex provider aliases = %#v", ids)
	}
	status := ProbeAgentAuthWithProviders(AgentCodex, AgentConfig{ModelProvider: CodexProviderOpenAIAPIKey}, t.TempDir(), nil, providers)
	if status.Authenticated || status.AuthKind != "" || status.AuthEvidence != "" || status.StoragePath != "" {
		t.Fatalf("Chat-only OpenAI override auth = %#v", status)
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

func TestProcessEnvDoesNotLeakUnselectedProviderKeyToCodex(t *testing.T) {
	clearHostEnv(t)
	manager := NewManager(nil, Config{
		Root: t.TempDir(),
		Env:  map[string]string{"UNUSED_PROVIDER_KEY": "must-not-leak"},
		Providers: map[string]modelprovider.ModelProviderConfig{
			"unused": {Type: "openai-compatible", BaseURL: "https://unused.test/v1", APIKeyEnv: "UNUSED_PROVIDER_KEY", Capabilities: []string{modelprovider.CapabilityResponses}},
		},
	}, nil)
	env := manager.processEnv(AgentCodex, AgentConfig{ModelProvider: modelprovider.ProviderOpenRouter})
	if env["UNUSED_PROVIDER_KEY"] != "" {
		t.Fatalf("unselected provider key leaked into Codex: %#v", env)
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
			"acme": {Type: "openai-compatible", BaseURL: "https://acme.test/v1", APIKeyEnv: "ACME_KEY", Capabilities: []string{modelprovider.CapabilityResponses}},
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
