package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

func TestAgentSettingsEnablesCodexOpenAIAPIKeyWithOpenAIProviderKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir()+"/codex-home")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_APIKEY", "")
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exe := testexec.Write(t, filepath.Join(root, "codex-acp"), "", "")
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store: store,
		Root:  root,
		AgentCatalog: acp.AgentCatalog{
			acp.AgentCodex: {
				Command:                 exe,
				ProviderMode:            acp.AgentProviderModeAgentDefaults,
				ModelProviderCapability: provider.CapabilityCodex,
				ModelProvider:           provider.ProviderOpenAI,
				Model:                   "gpt-5.4-mini",
			},
		},
	}).Handler()

	body := func(providerKeys map[string]any) *strings.Reader {
		return jsonReader(t, map[string]any{
			"acp": map[string]any{
				"codex": map[string]any{
					"enabled":        true,
					"command":        exe,
					"model_provider": acp.CodexProviderOpenAIAPIKey,
					"model":          "gpt-5.4-mini",
				},
			},
			"provider_keys": providerKeys,
		})
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", body(nil))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "cannot be enabled without authentication") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/v1/settings/agents", body(map[string]any{"openai": "openai-key"}))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAgentSettingsCodexOpenAIKeyOptionUsesOpenAIConfig(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		AgentCatalog: testACPAgentCatalog(nil),
		Providers: provider.StaticSource(map[string]provider.ModelProviderConfig{
			provider.ProviderOpenAI: {APIKey: "configured"},
		}),
	}).Handler()

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil)
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACPOptions map[string]struct {
			ModelProviders []struct {
				ID         string `json:"id"`
				Configured bool   `json:"configured"`
			} `json:"model_providers"`
		} `json:"acp_options"`
		Providers []struct {
			ID         string `json:"id"`
			Configured bool   `json:"configured"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if codexProviderConfigured(got.Providers, acp.CodexProviderOpenAIAPIKey) {
		t.Fatalf("openai api-key option must not leak into global providers: %#v", got.Providers)
	}
	if !codexProviderConfigured(got.ACPOptions[acp.AgentCodex].ModelProviders, acp.CodexProviderOpenAIAPIKey) {
		t.Fatalf("openai api-key option should inherit openai config: %#v", got.ACPOptions[acp.AgentCodex].ModelProviders)
	}
}

func codexProviderConfigured(providers []struct {
	ID         string `json:"id"`
	Configured bool   `json:"configured"`
}, id string) bool {
	for _, provider := range providers {
		if provider.ID == id {
			return provider.Configured
		}
	}
	return false
}
