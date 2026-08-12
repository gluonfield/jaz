package modelcapabilities

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
