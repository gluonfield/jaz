package acp

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

type codexMetadataCatalog struct {
	models []modelcatalog.Model
	err    error
	calls  int
}

func (c *codexMetadataCatalog) ProviderModels(string) ([]modelcatalog.Model, error) {
	c.calls++
	return c.models, c.err
}

func (*codexMetadataCatalog) AgentModels(string) []modelcatalog.Model {
	return nil
}

func TestResolveCodexModelMetadataUsesCanonicalCapabilities(t *testing.T) {
	catalog := &codexMetadataCatalog{models: []modelcatalog.Model{{
		Value:           "moonshotai/kimi-k3",
		Label:           "Kimi K3",
		Description:     "Agentic reasoning model",
		ContextLength:   1_048_576,
		InputModalities: []string{"text", "image"},
		Reasoning: modelcatalog.Reasoning{
			Status:        modelcatalog.ReasoningReady,
			Efforts:       []string{"low", "high", "max"},
			DefaultEffort: "max",
		},
	}}}
	manager := NewManager(nil, Config{ModelCatalog: catalog}, nil)
	cfg := AgentConfig{
		ProviderMode:  AgentProviderModeAgentDefaults,
		ModelProvider: provider.ProviderOpenRouter,
		Model:         "moonshotai/kimi-k3",
	}
	encoded, err := manager.resolveCodexModelMetadata(AgentCodex, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var metadata codexModelMetadata
	if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.ID != "moonshotai/kimi-k3" || metadata.ContextWindow != 1_048_576 ||
		metadata.DefaultReasoningEffort != "max" ||
		!reflect.DeepEqual(metadata.InputModalities, []string{"text", "image"}) ||
		!reflect.DeepEqual(metadata.ReasoningEfforts, []string{"low", "high", "max"}) {
		t.Fatalf("metadata = %#v", metadata)
	}
	if catalog.calls != 1 {
		t.Fatalf("provider catalog calls = %d, want 1", catalog.calls)
	}
	catalog.err = modelcatalog.ErrCatalogUnavailable
	if _, err := manager.resolveCodexModelMetadata(AgentCodex, cfg); !errors.Is(err, modelcatalog.ErrCatalogUnavailable) {
		t.Fatalf("catalog error = %v", err)
	}
}

func TestProcessEnvOnlySerializesProcessConfiguration(t *testing.T) {
	t.Setenv(codexModelMetadataEnv, `{"id":"stale"}`)
	catalog := &codexMetadataCatalog{err: errors.New("must not be called")}
	manager := NewManager(nil, Config{ModelCatalog: catalog}, nil)
	env := manager.processEnv(AgentCodex, AgentConfig{
		ProviderMode:  AgentProviderModeAgentDefaults,
		ModelProvider: provider.ProviderOpenRouter,
		Model:         "moonshotai/kimi-k3",
	})
	if got := env[codexModelMetadataEnv]; got != "" {
		t.Fatalf("Codex inherited model metadata %q", got)
	}
	if catalog.calls != 0 {
		t.Fatalf("process environment performed %d catalog calls", catalog.calls)
	}
}
