package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/providerstore"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func newProvidersTestServer(t *testing.T) (http.Handler, *sqlitestore.Store) {
	t.Helper()
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	source, err := provider.NewSource(map[string]provider.ModelProviderConfig{}, providerstore.Loader{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	return (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root, Providers: source}).Handler(), store
}

func TestProvidersCRUDLifecycle(t *testing.T) {
	handler, _ := newProvidersTestServer(t)

	// Create a custom provider with a key (loopback request → key setup allowed).
	body := `{"label":"Groq","base_url":"https://api.groq.com/openai/v1","api_type":"openai-compatible","api_key":"gk-test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/providers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", res.Code, res.Body)
	}
	var created struct {
		ID         string `json:"id"`
		APIKeyEnv  string `json:"api_key_env"`
		Configured bool   `json:"configured"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID != "groq" {
		t.Fatalf("id = %q", created.ID)
	}
	if created.APIKeyEnv != "JAZ_PROVIDER_GROQ_API_KEY" {
		t.Fatalf("api_key_env = %q", created.APIKeyEnv)
	}
	if !created.Configured {
		t.Fatal("expected configured=true after the key was set")
	}

	// The custom provider appears in the agent settings provider list, flagged custom.
	settingsRes := httptest.NewRecorder()
	handler.ServeHTTP(settingsRes, httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil))
	if settingsRes.Code != http.StatusOK {
		t.Fatalf("settings status %d", settingsRes.Code)
	}
	if !strings.Contains(settingsRes.Body.String(), `"id":"groq"`) {
		t.Fatalf("settings providers missing groq: %s", settingsRes.Body)
	}
	if !strings.Contains(settingsRes.Body.String(), `"custom":true`) {
		t.Fatalf("groq not flagged custom: %s", settingsRes.Body)
	}

	// Delete it.
	delReq := httptest.NewRequest(http.MethodDelete, "/v1/providers/groq", nil)
	delReq.RemoteAddr = "127.0.0.1:1234"
	delRes := httptest.NewRecorder()
	handler.ServeHTTP(delRes, delReq)
	if delRes.Code != http.StatusOK {
		t.Fatalf("delete status %d: %s", delRes.Code, delRes.Body)
	}

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/v1/providers", nil))
	if strings.Contains(listRes.Body.String(), "groq") {
		t.Fatalf("provider not deleted: %s", listRes.Body)
	}
}

func TestAgentSettingsTreatsLegacyLoopbackProviderAsNoKey(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	_, err = store.SaveSetting(providerstore.SettingsNamespace, providerstore.CustomKey, json.RawMessage(`{"providers":[{"id":"ollama-2","label":"Ollama","base_url":"http://localhost:11434/v1","api_type":"openai-compatible","api_key_env":"JAZ_PROVIDER_OLLAMA_2_API_KEY","created_at":"2026-07-04T14:11:38Z","updated_at":"2026-07-04T14:11:38Z"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	source, err := provider.NewSource(map[string]provider.ModelProviderConfig{}, providerstore.Loader{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root, Providers: source}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("settings status %d: %s", res.Code, res.Body)
	}
	var got struct {
		Providers []struct {
			ID             string `json:"id"`
			APIKeyEnv      string `json:"api_key_env"`
			RequiresAPIKey bool   `json:"requires_api_key"`
			Configured     bool   `json:"configured"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, provider := range got.Providers {
		if provider.ID != "ollama-2" {
			continue
		}
		if provider.APIKeyEnv != "" || provider.RequiresAPIKey || provider.Configured {
			t.Fatalf("legacy loopback provider exposed as key-backed: %#v", provider)
		}
		return
	}
	t.Fatalf("ollama-2 missing from providers: %#v", got.Providers)
}

func TestCreateProviderRejectsInvalidURL(t *testing.T) {
	handler, _ := newProvidersTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/providers",
		strings.NewReader(`{"label":"Bad","base_url":"ftp://x/v1"}`))
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", res.Code, res.Body)
	}
}
