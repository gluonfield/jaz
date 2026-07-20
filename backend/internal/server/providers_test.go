package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/providerstore"
	"github.com/wins/jaz/backend/internal/runtimeenv"
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

	body := `{"label":"Groq","base_url":"https://api.groq.com/openai/v1","capabilities":["chat_completions","responses"],"api_key":"gk-test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/providers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", res.Code, res.Body)
	}
	var created struct {
		ID           string   `json:"id"`
		APIKeyEnv    string   `json:"api_key_env"`
		Capabilities []string `json:"capabilities"`
		Configured   bool     `json:"configured"`
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
	if len(created.Capabilities) != 2 || created.Capabilities[0] != provider.CapabilityChatCompletions || created.Capabilities[1] != provider.CapabilityResponses {
		t.Fatalf("capabilities = %#v", created.Capabilities)
	}
	if !created.Configured {
		t.Fatal("expected configured=true after the key was set")
	}

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
	if !strings.Contains(settingsRes.Body.String(), `"capabilities":["chat_completions","responses"]`) {
		t.Fatalf("groq capabilities missing: %s", settingsRes.Body)
	}

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

func TestProviderDeleteRemovesKeyAfterLoopbackUpdate(t *testing.T) {
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
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root, Providers: source}).Handler()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/providers",
		strings.NewReader(`{"label":"Local","base_url":"https://llm.internal/v1","api_key":"secret"}`))
	createReq.RemoteAddr = "127.0.0.1:1234"
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", createRes.Code, createRes.Body)
	}
	if _, ok := runtimeenv.Lookup(runtimeenv.Path(root), "JAZ_PROVIDER_LOCAL_API_KEY"); !ok {
		t.Fatal("expected runtime provider key to be saved")
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/v1/providers/local",
		strings.NewReader(`{"label":"Local","base_url":"http://127.0.0.1:11434/v1"}`))
	updateReq.RemoteAddr = "127.0.0.1:1234"
	updateRes := httptest.NewRecorder()
	handler.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("update status %d: %s", updateRes.Code, updateRes.Body)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/providers/local", nil)
	deleteReq.RemoteAddr = "127.0.0.1:1234"
	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status %d: %s", deleteRes.Code, deleteRes.Body)
	}
	if _, ok := runtimeenv.Lookup(runtimeenv.Path(root), "JAZ_PROVIDER_LOCAL_API_KEY"); ok {
		t.Fatal("runtime provider key was not removed")
	}
}

func TestAgentSettingsTreatsLegacyLoopbackProviderAsNoKey(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	_, err = store.SaveSetting(providerstore.SettingsNamespace, providerstore.CustomKey, json.RawMessage(`{"providers":[{"id":"ollama-2","label":"Ollama","base_url":"http://127.0.0.1:1/v1","api_type":"openai-compatible","api_key_env":"JAZ_PROVIDER_OLLAMA_2_API_KEY","created_at":"2026-07-04T14:11:38Z","updated_at":"2026-07-04T14:11:38Z"}]}`))
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
		if provider.APIKeyEnv != "" || provider.RequiresAPIKey {
			t.Fatalf("legacy loopback provider exposed as key-backed: %#v", provider)
		}
		if !provider.Configured {
			t.Fatalf("legacy loopback provider should be config-ready: %#v", provider)
		}
		return
	}
	t.Fatalf("ollama-2 missing from providers: %#v", got.Providers)
}

func TestAgentSettingsDoesNotProbeNoKeyProviderStatus(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()
	handler, _ := newProviderStatusTestServer(t, upstream.URL+"/v1")

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("settings status %d: %s", res.Code, res.Body)
	}
	if hits.Load() != 0 {
		t.Fatalf("settings read probed provider %d times", hits.Load())
	}
}

func TestProviderStatusProbesNoKeyProviderConnected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()
	assertNoKeyProviderStatus(t, upstream.URL+"/v1", modelProviderStatusConnected)
}

func TestProviderStatusProbesNoKeyProviderDisconnected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	assertNoKeyProviderStatus(t, upstream.URL+"/v1", modelProviderStatusNotConnected)
}

func assertNoKeyProviderStatus(t *testing.T, baseURL string, status string) {
	t.Helper()
	handler, record := newProviderStatusTestServer(t, baseURL)

	settingsRes := httptest.NewRecorder()
	handler.ServeHTTP(settingsRes, httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil))
	if settingsRes.Code != http.StatusOK {
		t.Fatalf("settings status %d: %s", settingsRes.Code, settingsRes.Body)
	}
	var settingsGot struct {
		Providers []struct {
			ID             string `json:"id"`
			Configured     bool   `json:"configured"`
			RequiresAPIKey bool   `json:"requires_api_key"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(settingsRes.Body.Bytes(), &settingsGot); err != nil {
		t.Fatal(err)
	}
	for _, provider := range settingsGot.Providers {
		if provider.ID != record.ID {
			continue
		}
		if provider.RequiresAPIKey || !provider.Configured {
			t.Fatalf("provider settings = %#v", provider)
		}
		assertSingleProviderConnectionStatus(t, handler, record.ID, status)
		assertBatchProviderConnectionStatus(t, handler, record.ID, status)
		return
	}
	t.Fatalf("%s missing from providers: %#v", record.ID, settingsGot.Providers)
}

func assertSingleProviderConnectionStatus(t *testing.T, handler http.Handler, id string, status string) {
	t.Helper()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/providers/"+id+"/status", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body)
	}
	var got struct {
		ConnectionStatus string `json:"connection_status"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ConnectionStatus != status {
		t.Fatalf("connection_status = %q, want %q", got.ConnectionStatus, status)
	}
}

func assertBatchProviderConnectionStatus(t *testing.T, handler http.Handler, id string, status string) {
	t.Helper()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/providers/status", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("provider status %d: %s", res.Code, res.Body)
	}
	var got struct {
		Providers []struct {
			ID               string `json:"id"`
			ConnectionStatus string `json:"connection_status"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, provider := range got.Providers {
		if provider.ID != id {
			continue
		}
		if provider.ConnectionStatus != status {
			t.Fatalf("connection status = %q, want %q", provider.ConnectionStatus, status)
		}
		return
	}
	t.Fatalf("%s missing from provider statuses: %#v", id, got.Providers)
}

func newProviderStatusTestServer(t *testing.T, baseURL string) (http.Handler, providerstore.CustomProvider) {
	t.Helper()
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	record, err := providerstore.Create(store, providerstore.Input{Label: "Local", BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	source, err := provider.NewSource(map[string]provider.ModelProviderConfig{}, providerstore.Loader{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	return (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root, Providers: source}).Handler(), record
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
