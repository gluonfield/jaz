package acp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
)

func TestNativeMuseModelsPreserveCatalogAndUnknownCapabilities(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{agents: AgentCatalog{AgentMuse: {Env: map[string]string{
		"MUSE_CLI": executable, "MUSE_JAZ_TEST_CLI": root, "XDG_CONFIG_HOME": root,
	}}}}
	payload := `{"models":[
		{"modelId":"native-a","displayLabel":"Native A","contextLimit":123456,"isDefault":true,"variants":["minimal","max"],"defaultReasoningEffort":"max"},
		{"modelId":"native-b","displayLabel":"Native B","contextLimit":null,"variants":{"unknown":true}}
	]}`
	path := filepath.Join(root, "models.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := manager.DiscoverModels(t.Context(), AgentMuse)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 2 || catalog.DefaultModel != "native-a" {
		t.Fatalf("catalog = %#v", catalog)
	}
	known := catalog.Models[0]
	if known.Value != "native-a" || known.Label != "Native A" || known.ContextLength != 123456 ||
		known.Reasoning.DefaultEffort != "max" || !slices.Equal(known.Reasoning.Efforts, []string{"minimal", "max"}) {
		t.Fatalf("native metadata changed: %#v", known)
	}
	unknown := catalog.Models[1]
	if unknown.ContextLength != 0 || unknown.Reasoning.Status != modelcatalog.ReasoningUnavailable || len(unknown.Reasoning.Efforts) != 0 {
		t.Fatalf("unknown metadata was guessed: %#v", unknown)
	}
	if err := os.WriteFile(path, []byte(`{"models":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	empty, err := manager.DiscoverModels(t.Context(), AgentMuse)
	if err != nil || len(empty.Models) != 0 || empty.DefaultModel != "" {
		t.Fatalf("empty native catalog acquired fallback models: %#v, %v", empty, err)
	}
}
