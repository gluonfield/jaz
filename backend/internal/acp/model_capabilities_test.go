package acp

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

func TestModelCapabilitiesAddsCodexUltraWithoutInventingMinimal(t *testing.T) {
	catalog := capabilityCatalog{agents: map[string][]modelcatalog.Model{
		AgentCodex: {{
			Value:                 provider.OpenAIModelGPT56Sol,
			OpenRouterID:          "openai/gpt-5.6-sol",
			ReasoningEfforts:      []string{"low", "medium", "high", "xhigh", "max"},
			ReasoningEffortsKnown: true,
		}},
	}}
	capabilities := ModelCapabilities{Catalog: catalog}
	models := capabilities.AgentModels(AgentCodex)
	if got := strings.Join(models[0].ReasoningEfforts, ","); got != "low,medium,high,xhigh,max,ultra" {
		t.Fatalf("reasoning efforts = %q", got)
	}
	if models[0].ReasoningEffortScope != modelcatalog.ReasoningEffortScopeProvider {
		t.Fatalf("reasoning scope = %q", models[0].ReasoningEffortScope)
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
	if len(models) == 0 || !models[0].ReasoningEffortsKnown {
		t.Fatalf("models = %#v", models)
	}
	if got := strings.Join(models[0].ReasoningEfforts, ","); got != "none,minimal,low,medium,high,xhigh" {
		t.Fatalf("reasoning efforts = %q", got)
	}
	if models[0].ReasoningEffortScope != modelcatalog.ReasoningEffortScopeAgent {
		t.Fatalf("reasoning scope = %q", models[0].ReasoningEffortScope)
	}
}

func TestModelCapabilitiesLeavesProviderBackedModelsUnknownUntilCatalogLoads(t *testing.T) {
	models := (ModelCapabilities{Catalog: modelcatalog.NewService(nil)}).AgentModels(AgentCodex)
	if len(models) == 0 || models[0].ReasoningEffortsKnown || models[0].ReasoningEfforts != nil {
		t.Fatalf("models = %#v", models)
	}
}

func TestModelCapabilitiesAddsClaudeUltracodeOnlyToXhighModels(t *testing.T) {
	catalog := capabilityCatalog{agents: map[string][]modelcatalog.Model{
		AgentClaude: {
			{Value: "opus", ReasoningEfforts: []string{"low", "xhigh", "max"}, ReasoningEffortsKnown: true},
			{Value: "sonnet", ReasoningEfforts: []string{"low", "high", "max"}, ReasoningEffortsKnown: true},
		},
	}}
	models := (ModelCapabilities{Catalog: catalog}).AgentModels(AgentClaude)
	if got := strings.Join(models[0].ReasoningEfforts, ","); got != "low,xhigh,max,ultracode" {
		t.Fatalf("opus efforts = %q", got)
	}
	if got := strings.Join(models[1].ReasoningEfforts, ","); got != "low,high,max" {
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

func (c capabilityCatalog) CuratedAgentModelsForProvider(agent, _ string) ([]modelcatalog.Model, error) {
	return c.AgentModels(agent), nil
}

func (c capabilityCatalog) ProviderModels(providerID string) ([]modelcatalog.Model, error) {
	return append([]modelcatalog.Model(nil), c.providers[providerID]...), nil
}
