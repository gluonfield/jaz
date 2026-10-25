package acp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
)

func TestKimiDiscoveryUsesSelectedNativeProfileAndRefreshes(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	home := filepath.Join(root, "acp", "kimi")
	writeKimiCredential(t, home, `{"access_token":"test"}`)
	existing := t.TempDir()
	writeKimiCredential(t, existing, `{"access_token":"existing"}`)
	writeKimiConfig(t, existing)
	path := filepath.Join(home, "config.toml")
	write := func(config string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manager := &Manager{cfg: Config{Root: root, ModelCatalog: modelcatalog.NewService(nil)}, agents: AgentCatalog{AgentKimi: {
		Model: "saved-choice", Env: map[string]string{"KIMI_CODE_HOME": existing},
	}}}
	write(`default_model = "native/z"
[models."native/z"]
model = "wire-z"
display_name = "Native Z"
max_context_size = 123456
[models."native/a"]
model = "wire-a"
`)
	catalog, err := manager.DiscoverModels(t.Context(), AgentKimi)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 2 || catalog.DefaultModel != "native/z" {
		t.Fatalf("catalog = %#v", catalog)
	}
	known := catalog.Models[1]
	if known.Value != "native/z" || known.Label != "Native Z" || known.ContextLength != 123456 {
		t.Fatalf("native metadata changed: %#v", known)
	}
	unknown := catalog.Models[0]
	if unknown.Value != "native/a" || unknown.Label != "wire-a" || unknown.ContextLength != 0 ||
		unknown.Reasoning.Status != modelcatalog.ReasoningUnavailable || len(unknown.Reasoning.Efforts) != 0 {
		t.Fatalf("native metadata changed or guessed: %#v", unknown)
	}
	write(`[models.new]
model = "new-wire"
`)
	refreshed, err := manager.DiscoverModels(t.Context(), AgentKimi)
	if err != nil || len(refreshed.Models) != 1 || refreshed.Models[0].Value != "new" || refreshed.DefaultModel != "" {
		t.Fatalf("refresh = %#v, %v", refreshed, err)
	}
	write("")
	empty, err := manager.DiscoverModels(t.Context(), AgentKimi)
	if err != nil || empty.Models == nil || len(empty.Models) != 0 || empty.DefaultModel != "" {
		t.Fatalf("empty profile acquired fallback: %#v, %v", empty, err)
	}
	write("malformed = [toml")
	if _, err := manager.DiscoverModels(t.Context(), AgentKimi); err == nil {
		t.Fatal("malformed native profile silently succeeded")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.DiscoverModels(t.Context(), AgentKimi); err == nil {
		t.Fatal("missing native profile silently succeeded")
	}
	cfg := manager.agents.(AgentCatalog)[AgentKimi]
	cfg.Auth = AgentAuthConfig{Mode: AuthModeExistingCLI, Path: existing}
	manager.agents = AgentCatalog{AgentKimi: cfg}
	other, err := manager.DiscoverModels(t.Context(), AgentKimi)
	if err != nil || len(other.Models) != 1 || other.DefaultModel != "kimi-code/kimi-for-coding" {
		t.Fatalf("explicit existing profile = %#v, %v", other, err)
	}
}
