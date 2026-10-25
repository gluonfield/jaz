package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
)

func newKimiCatalogManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	clearHostEnv(t)
	home := filepath.Join(root, "acp", "kimi")
	writeKimiCredential(t, home, `{"access_token":"test"}`)
	writeKimiConfig(t, home)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(nil, Config{Root: root, ModelCatalog: modelcatalog.NewService(nil), AgentSource: AgentCatalog{AgentKimi: {
		Command: executable, Args: []string{"-test.run=^TestKimiNativeCatalogProcess$"},
		Model: "saved-choice", Env: map[string]string{"JAZ_TEST_KIMI_CATALOG": root},
	}}}, nil)
	t.Cleanup(manager.Close)
	return manager, root
}

func TestKimiDiscoveryUsesNativeChoicesAndDeletesProbe(t *testing.T) {
	manager, root := newKimiCatalogManager(t)
	catalog, err := manager.DiscoverModels(t.Context(), AgentKimi)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 3 || catalog.DefaultModel != "native-z" {
		t.Fatalf("catalog = %#v", catalog)
	}
	z, a, plain := catalog.Models[0], catalog.Models[1], catalog.Models[2]
	if z.Value != "native-z" || z.Label != "Effective Z" || z.ContextLength != 0 || !z.Reasoning.Mandatory ||
		!slices.Equal(z.Reasoning.Efforts, []string{"low", "high", "max"}) {
		t.Fatalf("native model metadata changed: %#v", z)
	}
	if a.Value != "native-a" || !slices.Equal(a.Reasoning.Efforts, []string{"off", "on"}) || a.Reasoning.Mandatory {
		t.Fatalf("previous model's effort leaked into choices: %#v", a)
	}
	if plain.Value != "native-plain" || plain.Reasoning.Status != modelcatalog.ReasoningUnavailable {
		t.Fatalf("invented plain-model controls: %#v", plain)
	}
	if _, err := os.Stat(filepath.Join(root, "live")); !os.IsNotExist(err) {
		t.Fatalf("discovery session was not deleted: %v", err)
	}
	requests, err := os.ReadFile(filepath.Join(root, "requests.jsonl"))
	if err != nil || strings.Contains(string(requests), "session/prompt") {
		t.Fatalf("discovery requested inference: %s, %v", requests, err)
	}
	if err := os.WriteFile(filepath.Join(root, "mode"), []byte("empty"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty, err := manager.DiscoverModels(t.Context(), AgentKimi)
	if err != nil || empty.Models == nil || len(empty.Models) != 0 || empty.DefaultModel != "" {
		t.Fatalf("empty native list acquired fallback: %#v, %v", empty, err)
	}
}

func TestKimiDiscoveryCleansUpFailuresAndCancellation(t *testing.T) {
	for _, mode := range []string{"no-delete", "fail-switch", "hold-switch", "fail-delete"} {
		t.Run(mode, func(t *testing.T) {
			manager, root := newKimiCatalogManager(t)
			if err := os.WriteFile(filepath.Join(root, "mode"), []byte(mode), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if mode == "hold-switch" {
				go func() {
					for ctx.Err() == nil {
						if _, err := os.Stat(filepath.Join(root, "switching")); err == nil {
							cancel()
							return
						}
						time.Sleep(time.Millisecond)
					}
				}()
			}
			if _, err := manager.DiscoverModels(ctx, AgentKimi); err == nil {
				t.Fatal("native discovery failure was hidden")
			}
			if mode != "fail-delete" {
				if _, err := os.Stat(filepath.Join(root, "live")); !os.IsNotExist(err) {
					t.Fatalf("probe leaked after %s: %v", mode, err)
				}
			}
		})
	}
}

func TestKimiConfiguredEffortUsesNativeThinkingContract(t *testing.T) {
	manager, root := newKimiCatalogManager(t)
	cfg, _, err := manager.agents.AgentConfig(AgentKimi)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Model = "native-z"
	cfg.ReasoningEffort = "max"
	manager.agents = AgentCatalog{AgentKimi: cfg}
	_, configured, effort, err := manager.spawnConfig(SpawnRequest{ACPAgent: AgentKimi})
	if err != nil || effort != "max" {
		t.Fatalf("native effort was rejected before session negotiation: %q, %v", effort, err)
	}
	ac, err := manager.connect(t.Context(), AgentKimi, configured, root, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ac.close()
	raw, err := ac.peer.Call(t.Context(), acpschema.AgentMethodSessionNew, acpschema.NewSessionRequest{Cwd: root, MCPServers: []acpschema.MCPServer{}})
	if err != nil {
		t.Fatal(err)
	}
	var response acpschema.NewSessionResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	session := newACPSessionInfo(raw, response)
	if _, err := manager.configuredModeState(t.Context(), ac, AgentKimi, session, configured); err != nil {
		t.Fatal(err)
	}
	requests, err := os.ReadFile(filepath.Join(root, "requests.jsonl"))
	if err != nil || !strings.Contains(string(requests), `"configId":"thinking","sessionId":"catalog-probe","value":"max"`) {
		t.Fatalf("native thinking selection did not reach the adapter: %s, %v", requests, err)
	}
	for _, choice := range []struct{ model, effort string }{{"native-z", "invalid"}, {"native-plain", "high"}} {
		configured.Model, configured.ReasoningEffort = choice.model, choice.effort
		if _, err := manager.configuredModeState(t.Context(), ac, AgentKimi, session, configured); err == nil {
			t.Fatalf("unsupported native effort silently accepted: %#v", choice)
		}
	}
	manager.agents = AgentCatalog{AgentKimi: cfg}
	_, _, cleared, err := manager.spawnConfig(SpawnRequest{ACPAgent: AgentKimi, ReasoningEffort: "none"})
	if err != nil || cleared != "" {
		t.Fatalf("clear-effort request = %q, %v", cleared, err)
	}
}
