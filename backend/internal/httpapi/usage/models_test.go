package usage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	usagecore "github.com/wins/jaz/backend/internal/usage"
)

func TestModelsHandlerReturnsModelBreakdown(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:          "usage-models",
		Runtime:       storage.RuntimeACP,
		RuntimeRef:    &storage.RuntimeRef{Agent: "codex"},
		ModelProvider: "openai",
		Model:         "gpt-5.4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(session.ID, storage.Usage{
		InputTokens:       12,
		CachedInputTokens: 30,
		OutputTokens:      4,
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/models?days=1&timezone=UTC", nil)
	res := httptest.NewRecorder()

	NewModelsHandler(usagecore.NewService(store)).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Models []struct {
			Agent         string           `json:"agent"`
			ModelProvider string           `json:"model_provider"`
			Model         string           `json:"model"`
			Usage         map[string]int64 `json:"usage"`
			SessionCount  int              `json:"session_count"`
		} `json:"models"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 {
		t.Fatalf("models = %#v", got.Models)
	}
	model := got.Models[0]
	if model.Agent != "codex" || model.ModelProvider != "openai" || model.Model != "gpt-5.4" || model.SessionCount != 1 {
		t.Fatalf("model = %#v", model)
	}
	if model.Usage["input_tokens"] != 12 ||
		model.Usage["cached_input_tokens"] != 30 ||
		model.Usage["output_tokens"] != 4 ||
		model.Usage["input_output_tokens"] != 16 {
		t.Fatalf("usage = %#v", model.Usage)
	}
	if _, ok := model.Usage["total_tokens"]; ok {
		t.Fatalf("model response must not overload total_tokens: %#v", model.Usage)
	}
}

func TestModelsHandlerRejectsInvalidDays(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/models?days=0", nil)
	res := httptest.NewRecorder()

	NewModelsHandler(usagecore.Service{}).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}
