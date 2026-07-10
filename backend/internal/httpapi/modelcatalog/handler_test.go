package modelcatalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	catalog "github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

func TestProviderModelsReturnsUnavailableWhenCatalogIsNotWarm(t *testing.T) {
	handler := NewHandler(catalog.NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
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

func TestProviderModelsScopesReasoningToAgent(t *testing.T) {
	handler := NewHandler(catalog.NewService(nil))
	req := httptest.NewRequest(http.MethodGet, "/v1/model-providers/openai/models?agent=%20Codex%20", nil)
	req.SetPathValue("provider", " OpenAI ")
	res := httptest.NewRecorder()

	handler.ProviderModels(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body providerModelsResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Models) == 0 || !slices.Contains(body.Models[0].ReasoningEfforts, "ultra") {
		t.Fatalf("models = %#v", body.Models)
	}
}
