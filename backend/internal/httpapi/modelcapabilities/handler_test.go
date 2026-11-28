package modelcapabilities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

func TestProviderModelsReturnsUnavailableWhenCatalogIsNotWarm(t *testing.T) {
	handler := NewHandler(modelcatalog.NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {},
	})))
	req := httptest.NewRequest(http.MethodGet, "/v1/model-providers/openrouter/models", nil)
	req.SetPathValue("provider", provider.ProviderOpenRouter)
	res := httptest.NewRecorder()

	handler.ProviderModels(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

// Codex owns its reasoning capabilities on its own backend, so a cold provider
// catalog must not hide them. Agent and provider arrive untrimmed from the query.
func TestProviderModelsServesAgentCapabilitiesBeforeReasoningCatalogIsWarm(t *testing.T) {
	handler := NewHandler(modelcatalog.NewService(nil))
	req := httptest.NewRequest(http.MethodGet, "/v1/model-providers/openai/models?agent=%20Codex%20", nil)
	req.SetPathValue("provider", " OpenAI ")
	res := httptest.NewRecorder()

	handler.ProviderModels(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body providerModelsResponse
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Models) == 0 {
		t.Fatal("no models returned")
	}
	for _, model := range body.Models {
		if model.Reasoning.Status != modelcatalog.ReasoningReady ||
			model.Reasoning.Scope != acp.ReasoningScopeAgent ||
			len(model.Reasoning.Efforts) == 0 {
			t.Fatalf("%s reasoning = %#v", model.Value, model.Reasoning)
		}
	}
}

type blockingModelSource struct {
	slowStarted chan struct{}
	releaseSlow chan struct{}
	slowCalls   atomic.Int32
}

func (s *blockingModelSource) DiscoverModels(ctx context.Context, agent string) (acp.AgentModelCatalog, error) {
	if agent == acp.AgentAntigravity {
		if s.slowCalls.Add(1) == 1 {
			close(s.slowStarted)
		}
		select {
		case <-s.releaseSlow:
			return acp.AgentModelCatalog{Models: []acp.AgentModel{{Value: "gemini-4-neon", Label: "Gemini 4 Neon"}}}, nil
		case <-ctx.Done():
			return acp.AgentModelCatalog{}, ctx.Err()
		}
	}
	return acp.AgentModelCatalog{Models: []acp.AgentModel{{Value: "gpt-6.1-sol", Label: "GPT-6.1 Sol"}}}, nil
}

func TestAgentModelsHandlerCoalescesSameAgentAndNeverBlocksOtherAgents(t *testing.T) {
	source := &blockingModelSource{
		slowStarted: make(chan struct{}),
		releaseSlow: make(chan struct{}),
	}
	handler := NewAgentModelsHandler(source)

	var wg sync.WaitGroup
	slowResponses := make([]*httptest.ResponseRecorder, 2)
	for i := range slowResponses {
		slowResponses[i] = httptest.NewRecorder()
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/v1/acp/agents/antigravity/models", nil)
			req.SetPathValue("agent", acp.AgentAntigravity)
			handler.ServeHTTP(slowResponses[idx], req)
		}(i)
	}

	<-source.slowStarted
	// A slow Antigravity discovery must not block Codex model discovery.
	fastReq := httptest.NewRequest(http.MethodGet, "/v1/acp/agents/codex/models", nil)
	fastReq.SetPathValue("agent", acp.AgentCodex)
	fastRes := httptest.NewRecorder()
	fastStart := time.Now()
	handler.ServeHTTP(fastRes, fastReq)
	if fastRes.Code != http.StatusOK || time.Since(fastStart) > 500*time.Millisecond {
		t.Fatalf("fast codex discovery failed or blocked: status=%d elapsed=%v", fastRes.Code, time.Since(fastStart))
	}

	// Give the second antigravity request a moment to attach to the in-flight call.
	time.Sleep(25 * time.Millisecond)
	close(source.releaseSlow)
	wg.Wait()

	if calls := source.slowCalls.Load(); calls != 1 {
		t.Fatalf("concurrent antigravity requests invoked DiscoverModels %d times, want 1", calls)
	}
	for i, res := range slowResponses {
		if res.Code != http.StatusOK {
			t.Fatalf("slow response %d status = %d, body = %s", i, res.Code, res.Body.String())
		}
	}
}
