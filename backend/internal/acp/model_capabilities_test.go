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
		Value:        provider.OpenAIModelGPT6Sol,
		OpenRouterID: "openai/gpt-6-sol",
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
	if err := capabilities.ValidateReasoningEffort(AgentCodex, provider.ProviderOpenAI, provider.OpenAIModelGPT6Sol, "minimal"); err == nil {
		t.Fatal("expected minimal to be rejected")
	}
	if err := capabilities.ValidateReasoningEffort(AgentCodex, provider.ProviderOpenAI, provider.OpenAIModelGPT6Sol, "ultra"); err != nil {
		t.Fatal(err)
	}
}

func TestCodexUltraModelsUseExplicitAllowlist(t *testing.T) {
	for _, test := range []struct {
		model string
		want  bool
	}{
		{provider.OpenAIModelGPT6Astra, true},
		{provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT6Astra, true},
		{provider.OpenAIModelGPT61Sol, true},
		{provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT61Sol, true},
		{provider.OpenAIModelGPT6Sol, true},
		{provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT6Sol, true},
		{"gpt-6-luna", false},
		{provider.ProviderOpenAI + "/gpt-6-luna", false},
	} {
		if got := isCodexUltraModel(modelcatalog.Model{Value: test.model}); got != test.want {
			t.Fatalf("isCodexUltraModel(%q) = %v, want %v", test.model, got, test.want)
		}
	}
	luna := modelcatalog.Model{
		Value: "gpt-6-luna",
		Reasoning: modelcatalog.Reasoning{
			Status:  modelcatalog.ReasoningReady,
			Efforts: []string{"max", "ultra"},
		},
	}
	models := (ModelCapabilities{Catalog: capabilityCatalog{agents: map[string][]modelcatalog.Model{AgentCodex: {luna}}}}).AgentModels(AgentCodex)
	if got := strings.Join(models[0].Reasoning.Efforts, ","); got != "max" {
		t.Fatalf("Luna reasoning efforts = %q", got)
	}
	spark := modelcatalog.Model{
		Value:     "gpt-5.3-codex-spark",
		Reasoning: modelcatalog.Reasoning{Status: modelcatalog.ReasoningUnavailable},
	}
	models = (ModelCapabilities{Catalog: capabilityCatalog{agents: map[string][]modelcatalog.Model{AgentCodex: {spark}}}}).AgentModels(AgentCodex)
	if containsString(models[0].Reasoning.Efforts, "ultra") {
		t.Fatalf("non-allowlisted fallback efforts = %v", models[0].Reasoning.Efforts)
	}
}

func TestModelCatalogRoutingByAgentAndProvider(t *testing.T) {
	for _, test := range []struct {
		agent     string
		provider  string
		agentOwns bool
		curated   bool
	}{
		{AgentCodex, "", true, true},
		{AgentClaude, "", true, true},
		{AgentCodex, provider.ProviderOpenAI, true, true},
		{AgentCodex, CodexProviderOpenAIAPIKey, true, true},
		{AgentCodex, provider.ProviderOpenRouter, false, true},
		{AgentCodex, provider.ProviderOllama, false, false},
		{AgentOpenCode, provider.ProviderOpenRouter, false, true},
		{AgentOpenCode, provider.ProviderOpenAI, false, false},
		{AgentClaude, provider.ProviderOpenAI, false, false},
	} {
		name := test.agent + "/" + test.provider
		if got := agentOwnsModelMetadata(test.agent, test.provider); got != test.agentOwns {
			t.Errorf("%s agentOwnsModelMetadata = %v, want %v", name, got, test.agentOwns)
		}
		if got := usesCuratedModels(test.agent, test.provider); got != test.curated {
			t.Errorf("%s usesCuratedModels = %v, want %v", name, got, test.curated)
		}
	}
}

func TestModelCapabilitiesUsesCodexHarnessForOpenAIModelsWithoutProviderMetadata(t *testing.T) {
	capabilities := ModelCapabilities{Catalog: modelcatalog.NewService(nil)}
	if err := capabilities.ValidateReasoningEffort(AgentCodex, provider.ProviderOpenAI, "gpt-5.3-codex-spark", "xhigh"); err != nil {
		t.Fatal(err)
	}
}

func TestModelCapabilitiesPopulatesNativeAgentReasoningCapabilities(t *testing.T) {
	capabilities := ModelCapabilities{Catalog: modelcatalog.NewService(nil)}
	models := capabilities.AgentModels(AgentCodex)
	if len(models) == 0 || models[0].Reasoning.Status != modelcatalog.ReasoningReady {
		t.Fatalf("models = %#v", models)
	}
	if !containsString(models[0].Reasoning.Efforts, "ultra") {
		t.Fatalf("%s efforts = %#v, want ultra", models[0].Value, models[0].Reasoning.Efforts)
	}
	if err := capabilities.ValidateReasoningEffort(AgentCodex, "", provider.OpenAIModelGPT6Sol, "ultra"); err != nil {
		t.Fatal(err)
	}
}

func TestModelCapabilitiesLeavesProviderBackedModelsUnknownUntilCatalogLoads(t *testing.T) {
	capabilities := ModelCapabilities{Catalog: modelcatalog.NewService(nil)}
	_, err := capabilities.ProviderModels(AgentCodex, provider.ProviderOpenRouter)
	if !errors.Is(err, modelcatalog.ErrCatalogUnavailable) {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
}

func TestOpenCodeModelCapabilitiesDoNotGuessAgentReasoningEfforts(t *testing.T) {
	for _, status := range []modelcatalog.ReasoningStatus{modelcatalog.ReasoningPending, modelcatalog.ReasoningUnavailable} {
		catalog := capabilityCatalog{agents: map[string][]modelcatalog.Model{AgentOpenCode: {{
			Value: "native-model", Reasoning: modelcatalog.Reasoning{Status: status},
		}}}}
		models := (ModelCapabilities{Catalog: catalog}).AgentModels(AgentOpenCode)
		if len(models) != 1 || models[0].Reasoning.Status != status || len(models[0].Reasoning.Efforts) != 0 {
			t.Fatalf("unreported %s controls became %#v", status, models)
		}
	}
	models := resolveModelCapabilities(AgentOpenCode, []modelcatalog.Model{{
		Value: "reported-model", Reasoning: modelcatalog.Reasoning{Status: modelcatalog.ReasoningReady, Efforts: []string{"none", "high"}},
	}}, true)
	if got := strings.Join(models[0].Reasoning.Efforts, ","); got != "none,high" {
		t.Fatalf("advertised controls changed: %s", got)
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

func TestModelCapabilitiesPreservesAutomaticProviderReasoning(t *testing.T) {
	models, err := (ModelCapabilities{Catalog: capabilityCatalog{providers: map[string][]modelcatalog.Model{
		provider.ProviderOllama: {{
			Value:     "qwen3.6:latest",
			Reasoning: modelcatalog.Reasoning{Status: modelcatalog.ReasoningReady, Automatic: true},
		}},
	}}}).ProviderModels(AgentCodex, provider.ProviderOllama)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || !models[0].Reasoning.Automatic || models[0].Reasoning.Scope != ReasoningScopeProvider {
		t.Fatalf("models = %#v", models)
	}
}

func TestModelCapabilitiesAddsCuratedAliasesToProviderModels(t *testing.T) {
	catalog := capabilityCatalog{
		agents: map[string][]modelcatalog.Model{AgentCodex: {{
			Value:        provider.OpenAIModelGPT6Sol,
			Label:        "GPT-6 Sol",
			OpenRouterID: "openai/gpt-6-sol",
		}}},
		providers: map[string][]modelcatalog.Model{provider.ProviderOpenRouter: {{
			Value: "openai/gpt-6-sol",
			Reasoning: modelcatalog.Reasoning{
				Status:  modelcatalog.ReasoningReady,
				Efforts: []string{"high"},
			},
		}}},
	}
	models, err := (ModelCapabilities{Catalog: catalog}).ProviderModels(AgentCodex, provider.ProviderOpenRouter)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || strings.Join(models[0].Aliases, ",") != "gpt-6-sol,GPT-6 Sol" {
		t.Fatalf("models = %#v", models)
	}
	if err := (ModelCapabilities{Catalog: catalog}).ValidateReasoningEffort(AgentCodex, provider.ProviderOpenRouter, provider.OpenAIModelGPT6Sol, "high"); err != nil {
		t.Fatal(err)
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
