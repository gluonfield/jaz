package modelcatalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
)

func TestServiceReturnsStartupOpenRouterCatalog(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v1/models" || r.URL.Query().Get("output_modalities") != "text,image" {
			t.Fatalf("unexpected upstream request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":[{
			"id":"anthropic/claude-sonnet-4.6",
			"name":"Anthropic: Claude Sonnet 4.6",
			"context_length":200000,
			"pricing":{"prompt":"0.000003","completion":"0.000015"},
			"reasoning":{"supported_efforts":["max","high","medium","low"],"default_effort":"medium"}
		}]}`))
	}))
	defer upstream.Close()

	service := NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {BaseURL: upstream.URL + "/api/v1"},
	}))
	if err := service.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		models, err := service.ProviderModels(provider.ProviderOpenRouter)
		if err != nil {
			t.Fatal(err)
		}
		if len(models) != 1 ||
			models[0].Value != "anthropic/claude-sonnet-4.6" ||
			models[0].Label != "Claude Sonnet 4.6" ||
			models[0].ContextLength != 200000 ||
			models[0].Pricing.Input != 0.000003 ||
			models[0].Pricing.Output != 0.000015 {
			t.Fatalf("unexpected models %#v", models)
		}
		if strings.Join(models[0].ReasoningEfforts, ",") != "low,medium,high,max" {
			t.Fatalf("reasoning efforts = %#v", models[0].ReasoningEfforts)
		}
	}
	if requests != 1 {
		t.Fatalf("upstream requests = %d, want 1", requests)
	}
}

func TestServiceDoesNotFetchOpenRouterOnProviderModels(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()

	service := NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {BaseURL: upstream.URL + "/api/v1"},
	}))
	if _, err := service.ProviderModels(provider.ProviderOpenRouter); err == nil {
		t.Fatal("expected missing startup catalog to fail")
	}
	if requests != 0 {
		t.Fatalf("upstream requests = %d, want 0", requests)
	}
}

func TestServiceWarmFetchesOpenRouterCatalogOnceUnderConcurrency(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()

	service := NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {BaseURL: upstream.URL + "/api/v1"},
	}))
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- service.Warm(context.Background())
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("upstream requests = %d, want 1", got)
	}
}

func TestServiceReturnsOpenAIBackendCatalog(t *testing.T) {
	models, err := NewService(nil).ProviderModels(codexOpenAIAPIKeyProvider)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) == 0 || models[0].Value != "gpt-5.5" {
		t.Fatalf("unexpected models %#v", models)
	}
	if models[0].ReasoningEfforts != nil {
		t.Fatalf("efforts before the catalog loads = %#v", models[0].ReasoningEfforts)
	}

	warmed := warmOpenRouterTestService(t, `{"data":[{
		"id":"openai/gpt-5.5",
		"name":"OpenAI: GPT-5.5",
		"reasoning":{"supported_efforts":["xhigh","high","medium","low","none"],"default_effort":"medium"}
	}]}`)
	models, err = warmed.ProviderModels(codexOpenAIAPIKeyProvider)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models[0].ReasoningEfforts, ",") != "none,low,medium,high,xhigh" {
		t.Fatalf("reasoning efforts = %#v", models[0].ReasoningEfforts)
	}
}

func TestServiceValidatesHarnessReasoningBeforeCatalogLoads(t *testing.T) {
	service := NewService(nil)
	if err := service.ValidateReasoningEffort("claude", "", "sonnet", "max"); err != nil {
		t.Fatal(err)
	}
	if err := service.ValidateReasoningEffort("claude", "", "sonnet", "minimal"); err == nil {
		t.Fatal("expected claude minimal reasoning to fail")
	}
	if err := service.ValidateReasoningEffort("codex", "openrouter", "openai/gpt-5.5", "xhigh"); err != nil {
		t.Fatal(err)
	}
	if err := service.ValidateReasoningEffort("codex", "openrouter", "openai/gpt-5.5", "max"); err == nil {
		t.Fatal("expected codex max reasoning to fail")
	}
}

func TestServiceAgentModelsFollowOpenRouterReasoning(t *testing.T) {
	service := warmOpenRouterTestService(t, `{"data":[
		{"id":"anthropic/claude-sonnet-5","name":"Anthropic: Claude Sonnet 5","reasoning":{"supported_efforts":["max","high","medium","low"],"default_effort":"medium"}},
		{"id":"anthropic/claude-opus-4.8","name":"Anthropic: Claude Opus 4.8","reasoning":{"mandatory":true,"supported_efforts":["max","xhigh","high","medium","low"],"default_effort":"medium"}},
		{"id":"anthropic/claude-haiku-4.5","name":"Anthropic: Claude Haiku 4.5","reasoning":{"mandatory":false}}
	]}`)

	models := service.AgentModels("claude")
	efforts := map[string]Model{}
	for _, model := range models {
		efforts[model.Value] = model
	}
	if strings.Join(efforts["sonnet"].ReasoningEfforts, ",") != "low,medium,high,max" {
		t.Fatalf("sonnet efforts = %#v", efforts["sonnet"].ReasoningEfforts)
	}
	if strings.Join(efforts["default"].ReasoningEfforts, ",") != "low,medium,high,xhigh,max,ultracode" {
		t.Fatalf("default efforts = %#v", efforts["default"].ReasoningEfforts)
	}
	if efforts["default"].ReasoningDefaultEffort != "medium" || !efforts["default"].ReasoningMandatory {
		t.Fatalf("default reasoning metadata = %#v", efforts["default"])
	}
	if efforts["haiku"].ReasoningEfforts == nil || len(efforts["haiku"].ReasoningEfforts) != 0 {
		t.Fatalf("haiku efforts = %#v", efforts["haiku"].ReasoningEfforts)
	}

	if err := service.ValidateReasoningEffort("claude", "", "sonnet", "xhigh"); err == nil {
		t.Fatal("expected xhigh to follow the live catalog and fail for sonnet")
	}
	if err := service.ValidateReasoningEffort("claude", "", "haiku", "low"); err == nil {
		t.Fatal("expected haiku reasoning to fail")
	}
	if err := service.ValidateReasoningEffort("claude", "", "default", "ultracode"); err != nil {
		t.Fatal(err)
	}
}

func TestServiceDoesNotBorrowProviderReasoningForHarnessAgents(t *testing.T) {
	service := warmOpenRouterTestService(t, `{"data":[{
		"id":"z-ai/glm-5.2",
		"name":"Z.AI: GLM 5.2",
		"reasoning":{"supported_efforts":["max","high","low"]}
	}]}`)

	if err := service.ValidateReasoningEffort("codex", "openrouter", "z-ai/glm-5.2", "low"); err != nil {
		t.Fatal(err)
	}
	if err := service.ValidateReasoningEffort("codex", "openrouter", "z-ai/glm-5.2", "max"); err == nil {
		t.Fatal("expected codex max reasoning to fail")
	}
}

func TestServiceFallsThroughStaticModelsWithoutReasoningToProviderCatalog(t *testing.T) {
	service := warmOpenRouterTestService(t, `{"data":[{
		"id":"openai/gpt-5.5",
		"name":"OpenAI: GPT-5.5",
		"reasoning":{"supported_efforts":["high","low"]}
	}]}`)

	if err := service.ValidateReasoningEffort("opencode", "openrouter", "openai/gpt-5.5", "high"); err != nil {
		t.Fatal(err)
	}
	if err := service.ValidateReasoningEffort("opencode", "openrouter", "openai/gpt-5.5", "max"); err == nil {
		t.Fatal("expected opencode max reasoning to fail for provider catalog without max")
	}
}

func TestServiceValidationFailsWhenOpenRouterCatalogIsUnavailable(t *testing.T) {
	service := NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {},
	}))

	err := service.ValidateReasoningEffort("opencode", "openrouter", "openai/gpt-5.5", "high")
	if !errors.Is(err, ErrCatalogUnavailable) {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
}

func warmOpenRouterTestService(t *testing.T, body string) *Service {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)
	service := NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {BaseURL: upstream.URL + "/api/v1"},
	}))
	if err := service.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	return service
}
