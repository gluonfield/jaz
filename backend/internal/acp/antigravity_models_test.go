package acp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
)

func fakeAntigravityCLI(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "agy"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.Link(binary, filepath.Join(root, name)); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestManagedAntigravityNeverUpdatesItself(t *testing.T) {
	for _, managed := range []bool{true, false} {
		root := fakeAntigravityCLI(t)
		if err := os.WriteFile(filepath.Join(root, "models"), []byte("native-a\tNative A\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := AgentConfig{LoginBinDir: root, Env: map[string]string{"JAZ_TEST_AGY_MODELS": root}}
		if managed {
			cfg.ManagedAdapter = AgentAntigravity
		}
		manager := &Manager{agents: AgentCatalog{AgentAntigravity: cfg}, cfg: Config{ModelCatalog: modelcatalog.NewService(nil)}}
		if _, err := manager.DiscoverModels(t.Context(), AgentAntigravity); err != nil {
			t.Fatal(err)
		}
		_, err := os.Stat(filepath.Join(root, "self-updated"))
		if updated := err == nil; updated == managed {
			t.Fatalf("managed %v: agy updated itself %v", managed, updated)
		}
	}
}

func TestAntigravityDiscoveryReadsNativeCatalogWithoutChangingSavedChoice(t *testing.T) {
	root := fakeAntigravityCLI(t)
	path := filepath.Join(root, "models")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source := AgentCatalog{AgentAntigravity: {
		Model: "saved-model", LoginBinDir: root,
		Env: map[string]string{"JAZ_TEST_AGY_MODELS": root},
	}}
	manager := &Manager{agents: source, cfg: Config{ModelCatalog: modelcatalog.NewService(nil)}}
	write(" neon-high\tNeon (High)\nneon-xhigh\tNeon (XHigh)\n\nneon-low\tNeon (Low)\nnative-z\tNative Z (High)\nnative-a\tNative A\nnative-unlabelled\n")
	catalog, err := manager.DiscoverModels(t.Context(), AgentAntigravity)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 4 || catalog.DefaultModel != "" {
		t.Fatalf("catalog = %#v", catalog)
	}
	neon := catalog.Models[0]
	if neon.Value != "neon" || neon.Label != "Neon" || neon.Reasoning.Status != modelcatalog.ReasoningReady ||
		!slices.Equal(neon.Reasoning.Efforts, []string{"low", "high", "xhigh"}) ||
		!slices.Equal(neon.Aliases, []string{"neon-high", "neon-xhigh", "neon-low"}) || neon.Reasoning.DefaultEffort != "" {
		t.Fatalf("levels of one model were not listed as its efforts: %#v", neon)
	}
	for i, want := range []struct{ id, label string }{
		{"native-z", "Native Z (High)"}, {"native-a", "Native A"}, {"native-unlabelled", "native-unlabelled"},
	} {
		got := catalog.Models[i+1]
		if got.Value != want.id || got.Label != want.label || got.ContextLength != 0 ||
			got.Reasoning.Status != modelcatalog.ReasoningUnavailable || len(got.Reasoning.Efforts) != 0 ||
			got.Reasoning.DefaultEffort != "" || got.Reasoning.Mandatory || got.Reasoning.Automatic {
			t.Fatalf("native metadata changed or invented: %#v", got)
		}
	}
	write("native-new\tNew model\n")
	refreshed, err := manager.DiscoverModels(t.Context(), AgentAntigravity)
	if err != nil || len(refreshed.Models) != 1 || refreshed.Models[0].Value != "native-new" {
		t.Fatalf("refresh = %#v, %v", refreshed, err)
	}
	write("")
	empty, err := manager.DiscoverModels(t.Context(), AgentAntigravity)
	if err != nil || empty.Models == nil || len(empty.Models) != 0 || empty.DefaultModel != "" {
		t.Fatalf("empty native list acquired fallback: %#v, %v", empty, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.DiscoverModels(t.Context(), AgentAntigravity); err == nil || strings.Contains(err.Error(), "private provider error") {
		t.Fatalf("failed native discovery must remain an error without provider stderr: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := manager.DiscoverModels(ctx, AgentAntigravity); err == nil {
		t.Fatal("cancelled discovery succeeded")
	}
}

func runFakeAntigravityModels(root string) int {
	if len(os.Args) != 2 || os.Args[1] != "models" {
		return 2
	}
	if os.Getenv("AGY_CLI_DISABLE_AUTO_UPDATE") != "true" {
		if err := os.WriteFile(filepath.Join(root, "self-updated"), nil, 0o600); err != nil {
			return 4
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "models"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "private provider error must not appear in the response")
		return 1
	}
	_, err = os.Stdout.Write(data)
	if err != nil {
		return 3
	}
	return 0
}
