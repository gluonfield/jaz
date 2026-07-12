package modelcapabilities

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestProviderModelsReturnsUnavailableBeforeReasoningCatalogIsWarm(t *testing.T) {
	handler := NewHandler(modelcatalog.NewService(nil))
	req := httptest.NewRequest(http.MethodGet, "/v1/model-providers/openai/models?agent=%20Codex%20", nil)
	req.SetPathValue("provider", " OpenAI ")
	res := httptest.NewRecorder()

	handler.ProviderModels(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}
