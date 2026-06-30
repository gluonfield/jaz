package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/provider"
)

func TestModelProviderModelsReturnsStartupOpenRouterCatalog(t *testing.T) {
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

	srv := &Server{
		Providers: provider.StaticSource(map[string]provider.ModelProviderConfig{
			provider.ProviderOpenRouter: {BaseURL: upstream.URL + "/api/v1"},
		}),
	}
	if err := srv.WarmModelProviderCatalogs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := srv.WarmModelProviderCatalogs(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := srv.Handler()

	for i := 0; i < 2; i++ {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/model-providers/openrouter/models", nil))
		if res.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
		}
		var got struct {
			Models []struct {
				Value            string   `json:"value"`
				Label            string   `json:"label"`
				ContextLength    int      `json:"context_length"`
				ReasoningEfforts []string `json:"reasoning_efforts"`
				Pricing          struct {
					Input  float64 `json:"input"`
					Output float64 `json:"output"`
				} `json:"pricing"`
			} `json:"models"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Models) != 1 ||
			got.Models[0].Value != "anthropic/claude-sonnet-4.6" ||
			got.Models[0].Label != "Claude Sonnet 4.6" ||
			got.Models[0].ContextLength != 200000 ||
			got.Models[0].Pricing.Input != 0.000003 ||
			got.Models[0].Pricing.Output != 0.000015 {
			t.Fatalf("unexpected models %#v", got.Models)
		}
		if joinStrings(got.Models[0].ReasoningEfforts) != "max,high,medium,low" {
			t.Fatalf("reasoning efforts = %#v", got.Models[0].ReasoningEfforts)
		}
	}
	if requests != 1 {
		t.Fatalf("upstream requests = %d, want 1", requests)
	}
}

func TestModelProviderModelsDoesNotFetchOpenRouterOnRequest(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()

	handler := (&Server{
		Providers: provider.StaticSource(map[string]provider.ModelProviderConfig{
			provider.ProviderOpenRouter: {BaseURL: upstream.URL + "/api/v1"},
		}),
	}).Handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/model-providers/openrouter/models", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if requests != 0 {
		t.Fatalf("upstream requests = %d, want 0", requests)
	}
}

func TestModelProviderModelsReturnsOpenAIBackendCatalog(t *testing.T) {
	handler := (&Server{}).Handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/model-providers/openai-api-key/models", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Models []struct {
			Value            string   `json:"value"`
			ReasoningEfforts []string `json:"reasoning_efforts"`
		} `json:"models"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Models) == 0 || got.Models[0].Value != "gpt-5.5" {
		t.Fatalf("unexpected models %#v", got.Models)
	}
	if joinStrings(got.Models[0].ReasoningEfforts) != "xhigh,high,medium,low,none" {
		t.Fatalf("reasoning efforts = %#v", got.Models[0].ReasoningEfforts)
	}
}

func joinStrings(values []string) string {
	out := ""
	for _, value := range values {
		if out != "" {
			out += ","
		}
		out += value
	}
	return out
}
