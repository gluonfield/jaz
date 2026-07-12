package acp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

	out, err := manager.AgentOptions(AgentOptionsRequest{Agent: AgentClaude, Name: "opus"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Agents) != 1 || out.Agents[0].Name != AgentClaude {
		t.Fatalf("agents = %#v", out.Agents)
	}
	options := out.Agents[0]
	if options.Name != AgentClaude || options.DefaultModel != "opus-4.8" || options.DefaultReasoningEffort != "xhigh" {
		t.Fatalf("defaults = %#v", options)
	}
	if len(options.Models) != 1 || options.Models[0].Model != "default" || options.Models[0].Label != "Opus 4.8" {
		t.Fatalf("models = %#v", options.Models)
	}
}

func TestAgentOptionsReportsModelScopedReasoningEfforts(t *testing.T) {
	manager := &Manager{
		cfg: Config{ModelCatalog: modelcatalog.NewService(nil)},
		agents: AgentCatalog{
			AgentGrok: {Model: modelcatalog.DefaultGrokModel},
		},
	}

	out, err := manager.AgentOptions(AgentOptionsRequest{Agent: AgentGrok})
	if err != nil {
		t.Fatal(err)
	}
	model := out.Agents[0].Models[0]
	if !model.ReasoningEffortsKnown || strings.Join(model.ReasoningEfforts, ",") != "none,minimal,low,medium,high,xhigh" {
		t.Fatalf("model reasoning = %#v", model)
	}
}

func TestAgentOptionsIncludesCuratedOpenRouterModelsWithoutNameFilter(t *testing.T) {
	manager := &Manager{
		cfg: Config{ModelCatalog: modelcatalog.NewService(nil)},
		agents: AgentCatalog{
			AgentOpenCode: {
				ProviderMode:            AgentProviderModeAgentDefaults,
				ModelProviderCapability: provider.CapabilityOpenCode,
				ModelProvider:           provider.ProviderOpenRouter,
				Model:                   provider.DefaultOpenRouterModel,
			},
		},
	}

	out, err := manager.AgentOptions(AgentOptionsRequest{Agent: AgentOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if out.Agents[0].DefaultModel != provider.DefaultOpenRouterModel {
		t.Fatalf("default model = %q", out.Agents[0].DefaultModel)
	}
	if len(out.Agents[0].Models) == 0 || out.Agents[0].Models[0].Model != provider.DefaultOpenRouterModel {
		t.Fatalf("first opencode model = %#v", out.Agents[0].Models)
	}
	models := modelValues(out.Agents[0].Models)
	for _, value := range []string{"deepseek/deepseek-v4-flash", "xiaomi/mimo-v2.5", "minimax/minimax-m3", provider.DefaultOpenRouterModel, "deepseek/deepseek-v4-pro", "tencent/hy3-preview", "stepfun/step-3.7-flash"} {
		if !models.contains(value) {
			t.Fatalf("curated opencode models missing %s: %#v", value, out.Agents[0].Models)
		}
	}
	for _, model := range out.Agents[0].Models {
		if strings.Contains(model.Model, "anthropic/") {
			t.Fatalf("anthropic model leaked into curated opencode list: %#v", out.Agents[0].Models)
		}
	}
}

func TestAgentOptionsScopesOpenCodeModelsToConfiguredProvider(t *testing.T) {
	manager := &Manager{
		cfg: Config{ModelCatalog: modelcatalog.NewService(nil)},
		agents: AgentCatalog{
			AgentOpenCode: {
				ProviderMode:            AgentProviderModeAgentDefaults,
				ModelProviderCapability: provider.CapabilityOpenCode,
				ModelProvider:           provider.ProviderOpenAI,
				Model:                   provider.DefaultOpenAIModel,
			},
		},
	}

	out, err := manager.AgentOptions(AgentOptionsRequest{Agent: AgentOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	models := modelValues(out.Agents[0].Models)
	if !models.contains(provider.DefaultOpenAIModel) {
		t.Fatalf("OpenAI model list missing default: %#v", out.Agents[0].Models)
	}
	if models.contains(provider.DefaultOpenRouterModel) {
		t.Fatalf("OpenCode/OpenAI advertised OpenRouter default: %#v", out.Agents[0].Models)
	}
}

func TestAgentOptionsSearchesOpenRouterWithNameFilter(t *testing.T) {
	catalog := warmOpenRouterCatalog(t)
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
				Model:                   provider.DefaultOpenRouterModel,
			},
		},
	}

	unfiltered, err := manager.AgentOptions(AgentOptionsRequest{Agent: AgentOpenCode})
	if err != nil {
		t.Fatal(err)
	}

	if modelValues(unfiltered.Agents[0].Models).contains("qwen/qwen3-coder") {
		t.Fatalf("unfiltered OpenRouter provider catalog leaked into agent_options: %#v", unfiltered.Agents[0].Models)
	}

	filtered, err := manager.AgentOptions(AgentOptionsRequest{Agent: AgentOpenCode, Name: "glm"})
	if err != nil {
		t.Fatal(err)
	}
	options := filtered.Agents[0]
	if options.ModelSearch == nil || options.ModelSearch.Provider != provider.ProviderOpenRouter {
		t.Fatalf("model search = %#v", options.ModelSearch)
	}
	if !modelValues(options.Models).contains(provider.DefaultOpenRouterModel) {
		t.Fatalf("filtered models = %#v", options.Models)
	}

	codex, err := manager.AgentOptions(AgentOptionsRequest{Agent: AgentCodex, Name: "glm"})
	if err != nil {
		t.Fatal(err)
	}
	if !modelValues(codex.Agents[0].Models).contains(provider.DefaultOpenRouterModel) {
		t.Fatalf("codex filtered models = %#v", codex.Agents[0].Models)
	}
	for _, model := range codex.Agents[0].Models {
		if model.Model == provider.DefaultOpenRouterModel && model.ModelProvider != provider.ProviderOpenRouter {
			t.Fatalf("codex OpenRouter model missing provider override: %#v", model)
		}
	}
}

func TestAgentOptionsSearchReturnsOpenRouterCatalogErrorWhenNoCuratedMatch(t *testing.T) {
	manager := &Manager{
		cfg: Config{ModelCatalog: modelcatalog.NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
			provider.ProviderOpenRouter: {},
		}))},
		agents: AgentCatalog{
			AgentOpenCode: {
				ProviderMode:            AgentProviderModeAgentDefaults,
				ModelProviderCapability: provider.CapabilityOpenCode,
				ModelProvider:           provider.ProviderOpenRouter,
				Model:                   provider.DefaultOpenRouterModel,
			},
		},
	}

	_, err := manager.AgentOptions(AgentOptionsRequest{Agent: AgentOpenCode, Name: "qwen"})
	if !errors.Is(err, modelcatalog.ErrCatalogUnavailable) {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
}

func warmOpenRouterCatalog(t *testing.T) *modelcatalog.Service {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"openai/gpt-5.6-sol","name":"OpenAI: GPT-5.6 Sol","reasoning":{"supported_efforts":["max","xhigh","high","medium","low"]}},
			{"id":"z-ai/glm-5.2","name":"Z.AI: GLM 5.2"},
			{"id":"qwen/qwen3-coder","name":"Qwen: Qwen3 Coder"},
			{"id":"anthropic/claude-sonnet-5","name":"Anthropic: Claude Sonnet 5","reasoning":{"supported_efforts":["max","high","medium","low"]}}
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

type modelValues []AgentModelOption

func (models modelValues) contains(value string) bool {
	for _, model := range models {
		if model.Model == value {
			return true
		}
	}
	return false
}
