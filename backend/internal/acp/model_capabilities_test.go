package acp

import (
	"errors"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

func TestModelCapabilitiesAddsCodexUltraWithoutInventingMinimal(t *testing.T) {
	model := modelcatalog.Model{
		Value:        provider.OpenAIModelGPT56Sol,
		OpenRouterID: "openai/gpt-5.6-sol",
		Reasoning: modelcatalog.Reasoning{
			Status:  modelcatalog.ReasoningReady,
			Efforts: []string{"low", "medium", "high", "xhigh", "max"},
		},
	}
	catalog := capabilityCatalog{
		agents:    map[string][]modelcatalog.Model{AgentCodex: {model}},
		providers: map[string][]modelcatalog.Model{provider.ProviderOpenAI: {model}},
	}
	capabilities := ModelCapabilities{Catalog: catalog}
	models := capabilities.AgentModels(AgentCodex)
	if got := strings.Join(models[0].Reasoning.Efforts, ","); got != "low,medium,high,xhigh,max,ultra" {
		t.Fatalf("reasoning efforts = %q", got)
	}
	if models[0].Reasoning.Scope != ReasoningScopeProvider {
		t.Fatalf("reasoning scope = %q", models[0].Reasoning.Scope)
	}
	if err := capabilities.ValidateReasoningEffort(AgentCodex, provider.ProviderOpenAI, provider.OpenAIModelGPT56Sol, "minimal"); err == nil {
		t.Fatal("expected minimal to be rejected")
	}
	if err := capabilities.ValidateReasoningEffort(AgentCodex, provider.ProviderOpenAI, provider.OpenAIModelGPT56Sol, "ultra"); err != nil {
		t.Fatal(err)
	}
}

func TestModelCapabilitiesUsesAgentScopedGrokEfforts(t *testing.T) {
	models := (ModelCapabilities{Catalog: modelcatalog.NewService(nil)}).AgentModels(AgentGrok)
	if len(models) == 0 || models[0].Reasoning.Status != modelcatalog.ReasoningReady {
		t.Fatalf("models = %#v", models)
	}
	if got := strings.Join(models[0].Reasoning.Efforts, ","); got != "none,minimal,low,medium,high,xhigh" {
		t.Fatalf("reasoning efforts = %q", got)
	}
	if models[0].Reasoning.Scope != ReasoningScopeAgent {
		t.Fatalf("reasoning scope = %q", models[0].Reasoning.Scope)
	}
}

func TestModelCapabilitiesUsesCodexHarnessForOpenAIModelsWithoutProviderMetadata(t *testing.T) {
	capabilities := ModelCapabilities{Catalog: modelcatalog.NewService(nil)}
	if err := capabilities.ValidateReasoningEffort(AgentCodex, provider.ProviderOpenAI, "gpt-5.3-codex-spark", "xhigh"); err != nil {
		t.Fatal(err)
	}
}

func TestModelCapabilitiesLeavesProviderBackedModelsUnknownUntilCatalogLoads(t *testing.T) {
	capabilities := ModelCapabilities{Catalog: modelcatalog.NewService(nil)}
	models := capabilities.AgentModels(AgentCodex)
	if len(models) == 0 || models[0].Reasoning.Status != modelcatalog.ReasoningPending {
		t.Fatalf("models = %#v", models)
	}
	if err := capabilities.ValidateReasoningEffort(AgentCodex, provider.ProviderOpenAI, provider.OpenAIModelGPT56Sol, "minimal"); !errors.Is(err, modelcatalog.ErrCatalogUnavailable) {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
}

func TestModelCapabilitiesValidatesSelectedProviderBeforeAgentCatalog(t *testing.T) {
	agentModel := modelcatalog.Model{Value: "shared", Reasoning: modelcatalog.Reasoning{Status: modelcatalog.ReasoningReady, Efforts: []string{"minimal"}}}
	providerModel := modelcatalog.Model{Value: "shared", Reasoning: modelcatalog.Reasoning{Status: modelcatalog.ReasoningReady, Efforts: []string{"high"}}}
	capabilities := ModelCapabilities{Catalog: capabilityCatalog{
		agents:    map[string][]modelcatalog.Model{AgentCodex: {agentModel}},
		providers: map[string][]modelcatalog.Model{"custom": {providerModel}},
	}}
	if err := capabilities.ValidateReasoningEffort(AgentCodex, "custom", "shared", "minimal"); err == nil {
		t.Fatal("expected selected provider capabilities to reject minimal")
	}
}

func TestModelCapabilitiesAddsClaudeUltracodeOnlyToXhighModels(t *testing.T) {
	catalog := capabilityCatalog{agents: map[string][]modelcatalog.Model{
		AgentClaude: {
			{Value: "opus", Reasoning: modelcatalog.Reasoning{Status: modelcatalog.ReasoningReady, Efforts: []string{"low", "xhigh", "max"}}},
			{Value: "sonnet", Reasoning: modelcatalog.Reasoning{Status: modelcatalog.ReasoningReady, Efforts: []string{"low", "high", "max"}}},
		},
	}}
	models := (ModelCapabilities{Catalog: catalog}).AgentModels(AgentClaude)
	if got := strings.Join(models[0].Reasoning.Efforts, ","); got != "low,xhigh,max,ultracode" {
		t.Fatalf("opus efforts = %q", got)
	}
	if got := strings.Join(models[1].Reasoning.Efforts, ","); got != "low,high,max" {
		t.Fatalf("sonnet efforts = %q", got)
	}
}

type capabilityCatalog struct {
	agents    map[string][]modelcatalog.Model
	providers map[string][]modelcatalog.Model
}

func (c capabilityCatalog) AgentModels(agent string) []modelcatalog.Model {
	return append([]modelcatalog.Model(nil), c.agents[agent]...)
}

func (c capabilityCatalog) ProviderModels(providerID string) ([]modelcatalog.Model, error) {
	return append([]modelcatalog.Model(nil), c.providers[providerID]...), nil
}
