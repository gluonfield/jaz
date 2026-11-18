package acp

import (
	"encoding/json"
	"testing"

	"github.com/wins/jaz/backend/internal/provider"
)

func TestExplicitProviderPreservesOpaqueModelIDsAcrossNormalizationAndConfig(t *testing.T) {
	for _, id := range []string{"markoff/gpt-oss-120b", "other/vendor/model", "gpt-oss-120b"} {
		t.Run(id, func(t *testing.T) {
			cfg := AgentConfig{ProviderMode: AgentProviderModeAgentDefaults, ModelProvider: "markoff", Model: id}
			cfg = cfg.NormalizeProviderModel("markoff").NormalizeProviderModel("markoff")
			if cfg.Model != id || cfg.ProviderQualifiedModel() != "markoff/"+id {
				t.Fatalf("model identity changed: %+v", cfg)
			}
			manager := NewManager(nil, Config{Root: t.TempDir(), Providers: map[string]provider.ModelProviderConfig{
				"markoff": {Type: "openai-compatible", BaseURL: "http://localhost:8080/v1", Models: []string{id}},
			}}, nil)
			env, err := manager.openCodeTestEnv(t, cfg)
			if err != nil {
				t.Fatal(err)
			}
			var content openCodeConfigContent
			if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
				t.Fatal(err)
			}
			if content.Model != "markoff/"+id || content.Provider["markoff"].Models[id].ID != id {
				t.Fatalf("native model changed in generated config: %+v", content)
			}
		})
	}
	legacy := AgentConfig{ProviderMode: AgentProviderModeAgentDefaults, Model: "markoff/vendor/model"}.NormalizeProviderModel("")
	if legacy.ModelProvider != "markoff" || legacy.Model != "vendor/model" {
		t.Fatalf("legacy combined model not restored: %+v", legacy)
	}
}
