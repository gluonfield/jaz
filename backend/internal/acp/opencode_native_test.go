package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestOpenCodeNativeProfilePreservesQualifiedModels(t *testing.T) {
	root := t.TempDir()
	cfg := AgentConfig{ProviderMode: AgentProviderModeAgentDefaults, ModelProvider: OpenCodeNativeProvider, Model: "zai-coding-plan/glm-5.3"}
	if got := cfg.NormalizeProviderModel(OpenCodeNativeProvider).ProviderQualifiedModel(); got != cfg.Model {
		t.Fatalf("qualified model = %q", got)
	}
	if got := resolveOpenCodeAuth(AgentAuthConfig{}, cfg, root, nil, nil); got.Authenticated {
		t.Fatal("missing profile was accepted")
	}
	profile := filepath.Join(root, ".local", "share", "opencode", "auth.json")
	if err := os.MkdirAll(filepath.Dir(profile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile, []byte(`{"opencode-go":{"type":"api","key":"dummy-go"},"zai-coding-plan":{"type":"api","key":"dummy-zai"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	auth := resolveOpenCodeAuth(AgentAuthConfig{}, cfg, root, nil, nil)
	if !auth.Authenticated || auth.Evidence != "opencode_native" || auth.APIKeyValue != "" {
		t.Fatalf("unexpected auth state: %#v", auth)
	}
	manager := NewManager(nil, Config{Root: root}, nil)
	env, err := manager.openCodeTestEnv(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := env["XDG_DATA_HOME"]; got != filepath.Join(root, ".local", "share") {
		t.Fatalf("XDG_DATA_HOME = %q", got)
	}
	var content struct {
		Model    string         `json:"model"`
		Provider map[string]any `json:"provider"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatal(err)
	}
	if content.Model != cfg.Model || len(content.Provider) != 0 {
		t.Fatalf("native config changed: %#v", content)
	}
}

func TestOpenCodeNativeRejectsEmptyProfileAndKeepsNativeModelOnResume(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, ".local", "share", "opencode", "auth.json")
	if err := os.MkdirAll(filepath.Dir(profile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile, []byte(`{"opencode-go":{"type":"api","key":""}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := AgentConfig{ProviderMode: AgentProviderModeAgentDefaults, ModelProvider: OpenCodeNativeProvider}
	if resolveOpenCodeAuth(AgentAuthConfig{}, cfg, root, nil, nil).Authenticated {
		t.Fatal("empty API key accepted")
	}
	cfg.Model = "opencode-go/glm-5.3-flash"
	selected := "zai-coding-plan/glm-5.3"
	restoreConfiguredChoices(&cfg, []sessionevents.Event{{Type: sessionevents.TypeAgentSession, AgentSession: &sessionevents.AgentSession{
		ConfigOptions: []sessionevents.AgentConfigOption{{ID: "model", Category: "model", CurrentValue: selected, UserValue: &selected}},
	}}})
	cfg = cfg.NormalizeProviderModel(OpenCodeNativeProvider)
	if got := agentPolicyForAgent(AgentOpenCode).sessionConfigModel(cfg); got != "zai-coding-plan/glm-5.3" {
		t.Fatalf("resume model = %q", got)
	}
}

func TestOpenCodeNativeCatalogDoesNotUseUnrelatedProviderMetadata(t *testing.T) {
	capabilities := ModelCapabilities{}
	models, err := capabilities.AgentModelsForProvider(AgentOpenCode, OpenCodeNativeProvider)
	if err != nil || len(models) != 0 {
		t.Fatalf("native models = %v, %v", models, err)
	}
	models, err = capabilities.ProviderModels(AgentOpenCode, OpenCodeNativeProvider)
	if err != nil || len(models) != 0 {
		t.Fatalf("native provider models = %v, %v", models, err)
	}
}
