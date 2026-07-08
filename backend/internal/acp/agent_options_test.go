package acp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

func TestAgentOptionsIncludesConfiguredModelOptions(t *testing.T) {
	manager := &Manager{
		cfg: Config{ModelCatalog: modelcatalog.NewService(nil)},
		agents: AgentCatalog{
			AgentClaude: {Model: "opus-4.8", ReasoningEffort: "xhigh"},
		},
	}

	out := manager.AgentOptions(AgentOptionsRequest{Agent: AgentClaude, Name: "opus"})
	if len(out.Agents) != 1 || out.Agents[0] != AgentClaude {
		t.Fatalf("agents = %#v", out.Agents)
	}
	options := out.AgentOptions[AgentClaude]
	if options.DefaultModel != "opus-4.8" || options.DefaultEffort != "xhigh" {
		t.Fatalf("defaults = %#v", options)
	}
	if len(options.Models) != 1 || options.Models[0].Value != "default" || options.Models[0].Label != "Opus 4.8" {
		t.Fatalf("models = %#v", options.Models)
	}
}

func TestAgentOptionsSearchesOpenRouterOnlyWithNameFilter(t *testing.T) {
	catalog := warmAgentOptionsOpenRouterCatalog(t)
	manager := &Manager{
		cfg: Config{ModelCatalog: catalog},
		agents: AgentCatalog{
			AgentCodex: {
				ProviderMode:            AgentProviderModeAgentDefaults,
				ModelProviderCapability: provider.CapabilityCodex,
				ModelProvider:           provider.ProviderOpenAI,
				Model:                   "gpt-5.5",
			},
			AgentOpenCode: {
				ProviderMode:            AgentProviderModeAgentDefaults,
				ModelProviderCapability: provider.CapabilityOpenCode,
				ModelProvider:           provider.ProviderOpenRouter,
				Model:                   "openai/gpt-5.4-mini",
			},
		},
	}

	unfiltered := manager.AgentOptions(AgentOptionsRequest{Agent: AgentOpenCode})
	if modelValues(unfiltered.AgentOptions[AgentOpenCode].Models).contains("z-ai/glm-5.2") {
		t.Fatalf("unfiltered OpenRouter models leaked into agent_options: %#v", unfiltered.AgentOptions[AgentOpenCode].Models)
	}

	filtered := manager.AgentOptions(AgentOptionsRequest{Agent: AgentOpenCode, Name: "glm"})
	options := filtered.AgentOptions[AgentOpenCode]
	if !slices.Contains(options.ModelProviderIDs, provider.ProviderOpenRouter) {
		t.Fatalf("provider ids = %#v", options.ModelProviderIDs)
	}
	if !modelValues(options.Models).contains("z-ai/glm-5.2") {
		t.Fatalf("filtered models = %#v", options.Models)
	}

	codex := manager.AgentOptions(AgentOptionsRequest{Agent: AgentCodex, Name: "glm"})
	if !modelValues(codex.AgentOptions[AgentCodex].Models).contains("z-ai/glm-5.2") {
		t.Fatalf("codex filtered models = %#v", codex.AgentOptions[AgentCodex].Models)
	}
}

func warmAgentOptionsOpenRouterCatalog(t *testing.T) *modelcatalog.Service {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"z-ai/glm-5.2","name":"Z.AI: GLM 5.2"},
			{"id":"qwen/qwen3-coder","name":"Qwen: Qwen3 Coder"}
		]}`))
	}))
	t.Cleanup(upstream.Close)
	service := modelcatalog.NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {BaseURL: upstream.URL + "/api/v1"},
	}))
	if err := service.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	return service
}

type modelValues []modelcatalog.Model

func (models modelValues) contains(value string) bool {
	for _, model := range models {
		if model.Value == value {
			return true
		}
	}
	return false
}
