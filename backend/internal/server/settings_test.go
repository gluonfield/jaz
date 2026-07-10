package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/serverconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

func testACPAgentCatalog(extra map[string]acp.AgentConfig) acp.AgentCatalog {
	return acp.MergeAgents(acp.BuiltinAgents(), extra)
}

func warmedModelCatalog(t *testing.T) *modelcatalog.Service {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"anthropic/claude-sonnet-5","name":"Anthropic: Claude Sonnet 5","reasoning":{"supported_efforts":["max","xhigh","high","medium","low"],"default_effort":"medium"}},
			{"id":"anthropic/claude-haiku-4.5","name":"Anthropic: Claude Haiku 4.5","reasoning":{"mandatory":false}}
		]}`))
	}))
	t.Cleanup(upstream.Close)
	service := modelcatalog.NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {BaseURL: upstream.URL + "/api/v1"},
	}))
	if err := service.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestMCPServerSettingsAPI(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store}).Handler()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{
		"name":"Docs",
		"url":"https://mcp.example.com/mcp",
		"enabled":true,
		"bearer_token_env_var":"DOCS_TOKEN",
		"oauth":{"client_id":"docs-client","client_secret_env_var":"DOCS_OAUTH_SECRET"},
		"headers":[{"name":"X-Team","value":"platform"},{"name":"X-Secret","envvar":"DOCS_SECRET"}]
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		URL               string `json:"url"`
		Enabled           bool   `json:"enabled"`
		BearerTokenEnvVar string `json:"bearer_token_env_var"`
		Status            string `json:"status"`
		ToolCount         int    `json:"tool_count"`
		OAuth             struct {
			ClientID           string `json:"client_id"`
			ClientSecretEnvVar string `json:"client_secret_env_var"`
		} `json:"oauth"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "Docs" || created.URL != "https://mcp.example.com/mcp" ||
		!created.Enabled || created.BearerTokenEnvVar != "DOCS_TOKEN" ||
		created.OAuth.ClientID != "docs-client" || created.OAuth.ClientSecretEnvVar != "DOCS_OAUTH_SECRET" {
		t.Fatalf("created = %#v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/mcp/servers", nil)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}
	var listed struct {
		Servers []struct {
			ID      string `json:"id"`
			Headers []struct {
				Name   string `json:"name"`
				Value  string `json:"value"`
				EnvVar string `json:"envvar"`
			} `json:"headers"`
			OAuth struct {
				ClientID string `json:"client_id"`
			} `json:"oauth"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Servers) != 1 || listed.Servers[0].ID != created.ID ||
		len(listed.Servers[0].Headers) != 2 || listed.Servers[0].Headers[0].Value != "platform" ||
		listed.Servers[0].Headers[1].EnvVar != "DOCS_SECRET" ||
		listed.Servers[0].OAuth.ClientID != "docs-client" {
		t.Fatalf("listed = %#v", listed)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/"+created.ID+"/disable", nil)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, disableReq)
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	loaded, err := store.LoadMCPServer(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Enabled {
		t.Fatalf("server still enabled: %#v", loaded)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/mcp/servers/"+created.ID, nil)
	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}
	servers, err := store.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers after delete = %#v", servers)
	}
}

func TestAgentSettingsAPIControlsEnabledACPAgents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: warmedModelCatalog(t), Store: store, AgentCatalog: testACPAgentCatalog(nil)}).Handler()

	getReq := httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil)
	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
	var got struct {
		Providers []struct {
			ID      string `json:"id"`
			BaseURL string `json:"base_url"`
		} `json:"providers"`
		Agents []string `json:"agents"`
		ACP    map[string]struct {
			Enabled         bool   `json:"enabled"`
			ModelProvider   string `json:"model_provider"`
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
		} `json:"acp"`
		ACPOptions map[string]struct {
			ReasoningEfforts []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"reasoning_efforts"`
			Models []struct {
				Value            string   `json:"value"`
				ReasoningEfforts []string `json:"reasoning_efforts"`
			} `json:"models"`
			Local            bool     `json:"local"`
			ProviderMode     string   `json:"provider_mode"`
			ModelProviderIDs []string `json:"model_provider_ids"`
			ModelProviders   []struct {
				ID           string `json:"id"`
				Label        string `json:"label"`
				DefaultModel string `json:"default_model"`
				Configured   bool   `json:"configured"`
			} `json:"model_providers"`
			AuthProviderID string `json:"auth_provider_id"`
			SupportsAuth   bool   `json:"supports_auth"`
		} `json:"acp_options"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Agents, ",") != "codex,claude,grok,opencode,antigravity" {
		t.Fatalf("unexpected seeded settings %#v", got)
	}
	if !hasModelProvider(got.Providers, "openai", "https://api.openai.com/v1") ||
		!hasModelProvider(got.Providers, "openrouter", "https://openrouter.ai/api/v1") {
		t.Fatalf("providers = %#v", got.Providers)
	}
	if got.ACP["codex"].Enabled ||
		got.ACP["codex"].Model != provider.OpenAIModelGPT56Sol {
		t.Fatalf("unexpected codex defaults %#v", got.ACP["codex"])
	}
	if !hasModelReasoningEfforts(got.ACPOptions["codex"].Models, provider.OpenAIModelGPT56Sol, "none,minimal,low,medium,high,xhigh,max,ultra") ||
		!hasModelReasoningEfforts(got.ACPOptions["codex"].Models, provider.OpenAIModelGPT56Terra, "none,minimal,low,medium,high,xhigh,max,ultra") ||
		!hasModelReasoningEfforts(got.ACPOptions["codex"].Models, provider.OpenAIModelGPT56Luna, "none,minimal,low,medium,high,xhigh,max,ultra") {
		t.Fatalf("codex model options missing GPT-5.6 family %#v", got.ACPOptions["codex"].Models)
	}
	if got.ACPOptions["codex"].AuthProviderID != provider.ProviderOpenAI ||
		strings.Join(got.ACPOptions["codex"].ModelProviderIDs, ",") != "openai,openai-api-key,openrouter" {
		t.Fatalf("unexpected codex provider options %#v", got.ACPOptions["codex"])
	}
	if len(got.ACPOptions["codex"].ModelProviders) < 2 ||
		got.ACPOptions["codex"].ModelProviders[0].Label != "OpenAI OAuth" ||
		got.ACPOptions["codex"].ModelProviders[0].DefaultModel != acp.CodexOpenAIDefaultModel ||
		got.ACPOptions["codex"].ModelProviders[1].ID != acp.CodexProviderOpenAIAPIKey ||
		got.ACPOptions["codex"].ModelProviders[1].Label != "OpenAI API key" ||
		got.ACPOptions["codex"].ModelProviders[1].DefaultModel != acp.CodexOpenAIDefaultModel {
		t.Fatalf("unexpected codex model providers %#v", got.ACPOptions["codex"].ModelProviders)
	}
	if got.ACP["grok"].Enabled ||
		got.ACP["grok"].Model != modelcatalog.DefaultGrokModel {
		t.Fatalf("unexpected grok defaults %#v", got.ACP["grok"])
	}
	if !hasModelReasoningEfforts(got.ACPOptions["grok"].Models, modelcatalog.DefaultGrokModel, "") {
		t.Fatalf("grok model options missing default %#v", got.ACPOptions["grok"].Models)
	}
	if got.ACP["opencode"].Enabled ||
		got.ACP["opencode"].ModelProvider != "openrouter" ||
		got.ACP["opencode"].Model != "z-ai/glm-5.2" {
		t.Fatalf("unexpected opencode defaults %#v", got.ACP["opencode"])
	}
	if !hasReasoningEffort(got.ACPOptions["claude"].ReasoningEfforts, "ultracode") ||
		!hasReasoningEffort(got.ACPOptions["codex"].ReasoningEfforts, "max") ||
		!hasReasoningEffort(got.ACPOptions["codex"].ReasoningEfforts, "ultra") ||
		hasReasoningEffort(got.ACPOptions["codex"].ReasoningEfforts, "ultracode") {
		t.Fatalf("unexpected acp options %#v", got.ACPOptions)
	}
	if !hasModelReasoningEfforts(got.ACPOptions["claude"].Models, "sonnet", "low,medium,high,xhigh,max,ultracode") {
		t.Fatalf("claude model options missing sonnet reasoning matrix %#v", got.ACPOptions["claude"].Models)
	}
	// The desktop client treats a missing capability flag as "requires a command",
	// so a false flag must be emitted explicitly, never dropped by omitempty. A
	// struct unmarshal can't tell absent from false, so assert the raw keys exist.
	var rawOptions struct {
		ACPOptions map[string]map[string]json.RawMessage `json:"acp_options"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &rawOptions); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"local", "supports_auth"} {
		if _, ok := rawOptions.ACPOptions["codex"][flag]; !ok {
			t.Fatalf("acp_options.codex must emit %q, body = %s", flag, getRes.Body.String())
		}
	}
	reasoningEfforts, ok := rawOptions.ACPOptions["antigravity"]["reasoning_efforts"]
	if !ok || len(reasoningEfforts) == 0 || reasoningEfforts[0] != '[' {
		t.Fatalf("acp_options.antigravity.reasoning_efforts must be an array, body = %s", getRes.Body.String())
	}
	if got.ACPOptions["opencode"].ProviderMode != acp.AgentProviderModeAgentDefaults ||
		!hasString(got.ACPOptions["opencode"].ModelProviderIDs, "openrouter") ||
		!hasString(got.ACPOptions["opencode"].ModelProviderIDs, "openai") ||
		hasString(got.ACPOptions["opencode"].ModelProviderIDs, acp.CodexProviderOpenAIAPIKey) {
		t.Fatalf("unexpected opencode capabilities %#v", got.ACPOptions["opencode"])
	}

	putReq := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"acp":{
			"codex":{"enabled":true,"command":"/opt/jaz/codex-acp -c 'sandbox_mode=\"danger-full-access\"'","model":"gpt-5.5","reasoning_effort":"high"},
			"claude":{"enabled":false,"command":"npx -y @agentclientprotocol/claude-agent-acp@0.43.0","model":"default","reasoning_effort":"medium"}
		},
		"acp_keys":{"codex":"codex-key"}
	}`))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.RemoteAddr = "127.0.0.1:1234"
	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}
	loaded, err := store.LoadSetting("agents", "defaults")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loaded.Value), `"enabled":false`) ||
		!strings.Contains(string(loaded.Value), `"reasoning_effort":"high"`) {
		t.Fatalf("stored settings = %s", loaded.Value)
	}
	if strings.Contains(string(loaded.Value), `/opt/jaz/codex-acp`) ||
		strings.Contains(string(loaded.Value), `@agentclientprotocol/claude-agent-acp`) {
		t.Fatalf("managed agent command leaked into settings = %s", loaded.Value)
	}
	env, err := os.ReadFile(runtimeenv.Path(store.RootDir()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), `JAZ_ACP_CODEX_API_KEY="codex-key"`) {
		t.Fatalf("runtime env = %s", env)
	}
	var saved struct {
		ACPAuth map[string]struct {
			APIKeyConfigured bool   `json:"api_key_configured"`
			AuthKind         string `json:"auth_kind"`
		} `json:"acp_auth"`
	}
	if err := json.Unmarshal(putRes.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.ACPAuth["codex"].APIKeyConfigured || saved.ACPAuth["codex"].AuthKind != acp.AuthKindAPIKey {
		t.Fatalf("unexpected acp auth status %#v", saved.ACPAuth)
	}
}

func TestAgentSettingsSavesModelProviderKey(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

	putReq := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"provider_keys":{"openrouter":"sk-or-test"}
	}`))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.RemoteAddr = "127.0.0.1:1234"
	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}

	env, err := os.ReadFile(runtimeenv.Path(store.RootDir()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), `OPENROUTER_API_KEY="sk-or-test"`) {
		t.Fatalf("runtime env = %s", env)
	}

	var saved struct {
		Providers []struct {
			ID         string `json:"id"`
			Configured bool   `json:"configured"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(putRes.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	configured := false
	for _, provider := range saved.Providers {
		if provider.ID == "openrouter" {
			configured = provider.Configured
		}
	}
	if !configured {
		t.Fatalf("openrouter not reported configured: %#v", saved.Providers)
	}
}

func TestAgentSettingsRejectsEnabledACPWithoutAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir()+"/codex-home")
	t.Setenv("JAZ_ACP_CODEX_API_KEY", "")
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
			acp.AgentCodex: {Command: exe, Model: "gpt-5.4-mini"},
		},
	}).Handler()

	missingProfile := filepath.Join(root, "missing-codex")
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", jsonReader(t, map[string]any{
		"acp": map[string]any{
			"codex": map[string]any{
				"enabled": true,
				"command": exe,
				"model":   "gpt-5.4-mini",
				"auth": map[string]any{
					"mode": "jaz_profile",
					"path": missingProfile,
				},
			},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "cannot be enabled without authentication") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/v1/settings/agents", jsonReader(t, map[string]any{
		"acp": map[string]any{
			"codex": map[string]any{
				"enabled": true,
				"command": exe,
				"model":   "gpt-5.4-mini",
				"auth": map[string]any{
					"mode": "jaz_profile",
					"path": missingProfile,
				},
			},
		},
		"acp_keys": map[string]any{"codex": "codex-key"},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAgentSettingsRejectsEnabledProviderBackedACPWithoutProviderKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_APIKEY", "")
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exe := testexec.Write(t, filepath.Join(root, "opencode-acp"), "", "")
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store: store,
		Root:  root,
		AgentCatalog: acp.AgentCatalog{
			acp.AgentOpenCode: {
				Command:                 exe,
				ProviderMode:            acp.AgentProviderModeAgentDefaults,
				ModelProviderCapability: provider.CapabilityOpenCode,
				ModelProvider:           provider.ProviderOpenAI,
				Model:                   "gpt-5.4-mini",
			},
		},
	}).Handler()

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", jsonReader(t, map[string]any{
		"acp": map[string]any{
			"opencode": map[string]any{
				"enabled":        true,
				"command":        exe,
				"model_provider": "openai",
				"model":          "gpt-5.4-mini",
			},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "cannot be enabled without authentication") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/v1/settings/agents", jsonReader(t, map[string]any{
		"acp": map[string]any{
			"opencode": map[string]any{
				"enabled":        true,
				"command":        exe,
				"model_provider": "openai",
				"model":          "gpt-5.4-mini",
			},
		},
		"provider_keys": map[string]any{"openai": "openai-key"},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAgentSettingsSavesCustomModelProviderKey(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store: store,
		Root:  root,
		Providers: provider.StaticSource(map[string]provider.ModelProviderConfig{
			"internal": {
				Type:    "openai-compatible",
				BaseURL: "https://llm.internal/v1",
			},
		}),
	}).Handler()

	putReq := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"provider_keys":{"internal":"internal-key"}
	}`))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.RemoteAddr = "127.0.0.1:1234"
	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}

	env, err := os.ReadFile(runtimeenv.Path(store.RootDir()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), `JAZ_PROVIDER_INTERNAL_API_KEY="internal-key"`) {
		t.Fatalf("runtime env = %s", env)
	}
}

func TestAgentSettingsRejectsInvalidSettingsBeforeSavingACPKeys(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"acp":{"codex":{"enabled":true,"command":"codex-acp","model":"gpt-5.5","reasoning_effort":"medium","auth":{"mode":"broken"}}},
		"acp_keys":{"codex":"should-not-save"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()

	(&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "auth mode") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(runtimeenv.Path(root)); !os.IsNotExist(err) {
		t.Fatalf("runtime env should not be written, err = %v", err)
	}
}

func hasModelProvider(providers []struct {
	ID      string `json:"id"`
	BaseURL string `json:"base_url"`
}, id, baseURL string) bool {
	for _, provider := range providers {
		if provider.ID == id && provider.BaseURL == baseURL {
			return true
		}
	}
	return false
}

func hasReasoningEffort(options []struct {
	Value string `json:"value"`
	Label string `json:"label"`
}, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func hasModelReasoningEfforts(models []struct {
	Value            string   `json:"value"`
	ReasoningEfforts []string `json:"reasoning_efforts"`
}, value, efforts string) bool {
	for _, model := range models {
		if model.Value == value && strings.Join(model.ReasoningEfforts, ",") == efforts {
			return true
		}
	}
	return false
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func jsonReader(t *testing.T, value any) *strings.Reader {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(body))
}

func TestAgentSettingsAPIIncludesCustomProviderForACPAgents(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog := testACPAgentCatalog(acp.AgentCatalog{
		acp.AgentCodex: {
			Command:                 "codex",
			ProviderMode:            acp.AgentProviderModeAgentDefaults,
			ModelProviderCapability: provider.CapabilityCodex,
			ModelProvider:           "internal",
			Model:                   "gpt-5.4-mini",
		},
		acp.AgentOpenCode: {
			Command:                 "opencode",
			ProviderMode:            acp.AgentProviderModeAgentDefaults,
			ModelProviderCapability: provider.CapabilityOpenCode,
			ModelProvider:           "internal",
			Model:                   "chat",
		},
	})
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		AgentCatalog: catalog,
		Providers: provider.StaticSource(map[string]provider.ModelProviderConfig{
			"internal": {
				Type:    "openai-compatible",
				Label:   "Internal",
				BaseURL: "https://llm.internal/v1",
				APIKey:  "internal-key",
			},
		}),
	}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Providers []struct {
			ID               string `json:"id"`
			Label            string `json:"label"`
			BaseURL          string `json:"base_url"`
			Codex            bool   `json:"codex"`
			OpenCode         bool   `json:"opencode"`
			OpenAICompatible bool   `json:"openai_compatible"`
			Configured       bool   `json:"configured"`
		} `json:"providers"`
		ACPAuth map[string]struct {
			Authenticated bool   `json:"authenticated"`
			AuthKind      string `json:"auth_kind"`
		} `json:"acp_auth"`
		ACPOptions map[string]struct {
			ProviderMode     string   `json:"provider_mode"`
			ModelProviderIDs []string `json:"model_provider_ids"`
		} `json:"acp_options"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var custom *struct {
		ID               string `json:"id"`
		Label            string `json:"label"`
		BaseURL          string `json:"base_url"`
		Codex            bool   `json:"codex"`
		OpenCode         bool   `json:"opencode"`
		OpenAICompatible bool   `json:"openai_compatible"`
		Configured       bool   `json:"configured"`
	}
	for i := range got.Providers {
		if got.Providers[i].ID == "internal" {
			custom = &got.Providers[i]
			break
		}
	}
	if custom == nil || custom.Label != "Internal" || custom.BaseURL != "https://llm.internal/v1" ||
		!custom.Codex || !custom.OpenCode || !custom.OpenAICompatible || !custom.Configured {
		t.Fatalf("custom provider not exposed correctly: %#v", got.Providers)
	}
	if options := got.ACPOptions["codex"]; options.ProviderMode != acp.AgentProviderModeAgentDefaults ||
		!hasString(options.ModelProviderIDs, "internal") {
		t.Fatalf("codex capabilities lost: %#v", options)
	}
	if options := got.ACPOptions["opencode"]; options.ProviderMode != acp.AgentProviderModeAgentDefaults ||
		!hasString(options.ModelProviderIDs, "internal") {
		t.Fatalf("opencode capabilities lost: %#v", options)
	}
	if auth := got.ACPAuth["codex"]; !auth.Authenticated || auth.AuthKind != acp.AuthKindAPIKey {
		t.Fatalf("codex auth did not use custom provider config: %#v", auth)
	}
	if auth := got.ACPAuth["opencode"]; !auth.Authenticated || auth.AuthKind != acp.AuthKindAPIKey {
		t.Fatalf("opencode auth did not use custom provider config: %#v", auth)
	}
}

func TestAgentSettingsAPIRoundTripsConfiguredACPAgent(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog := testACPAgentCatalog(map[string]acp.AgentConfig{
		acp.AgentJaz: {
			Local: true,
		},
		"local_helper": {
			Command:         "/opt/jaz/local-helper",
			Args:            []string{"--stdio"},
			Model:           "helper-model",
			ReasoningEffort: "low",
		},
	})
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, AgentCatalog: catalog}).Handler()

	getReq := httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil)
	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
	var got struct {
		Agents []string `json:"agents"`
		ACP    map[string]struct {
			Enabled bool   `json:"enabled"`
			Model   string `json:"model"`
		} `json:"acp"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Agents, ",") != "codex,claude,grok,opencode,antigravity,local_helper" {
		t.Fatalf("agents = %#v", got.Agents)
	}
	if _, ok := got.ACP[acp.AgentJaz]; ok {
		t.Fatalf("jaz should not be exposed in editable settings: %#v", got.ACP)
	}
	if got.ACP["local_helper"].Model != "helper-model" {
		t.Fatalf("custom agent not seeded: %#v", got.ACP["local_helper"])
	}

	putReq := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"acp":{
			"codex":{"enabled":false,"model":"gpt-5.5","reasoning_effort":"medium"},
			"claude":{"enabled":false,"model":"default","reasoning_effort":"medium"},
			"local_helper":{"enabled":true,"model":"helper-model","reasoning_effort":"low"}
		}
	}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}
	if !strings.Contains(putRes.Body.String(), "local_helper") {
		t.Fatalf("custom agent missing from response: %s", putRes.Body.String())
	}
	if strings.Contains(putRes.Body.String(), `"jaz"`) {
		t.Fatalf("jaz leaked into response: %s", putRes.Body.String())
	}
}

func TestAgentSettingsRejectsJazACPAgent(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog := testACPAgentCatalog(map[string]acp.AgentConfig{
		acp.AgentJaz: {Local: true},
	})
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"acp":{"jaz":{"enabled":true}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, AgentCatalog: catalog}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest ||
		!strings.Contains(res.Body.String(), "unknown acp agent") ||
		!strings.Contains(res.Body.String(), "jaz") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAgentSettingsRejectUnknownACPAgent(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"acp":{"missing":{"enabled":true}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "unknown acp agent") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

// The renderer is a separate origin, so a DELETE is preflighted. The handler
// must advertise DELETE in Access-Control-Allow-Methods or the browser blocks
// the request before it ever reaches us.
func TestCORSAllowsDeletePreflight(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store}).Handler()

	req := httptest.NewRequest(http.MethodOptions, "/v1/mcp/servers/abc", nil)
	req.Header.Set("Origin", "http://localhost:5180")
	req.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d", res.Code)
	}
	if allow := res.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(allow, http.MethodDelete) {
		t.Fatalf("Access-Control-Allow-Methods = %q, missing DELETE", allow)
	}
	if allow := res.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(allow, "Authorization") {
		t.Fatalf("Access-Control-Allow-Headers = %q, missing Authorization", allow)
	}
	if allow := res.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(allow, "X-Jaz-Client-Platform") {
		t.Fatalf("Access-Control-Allow-Headers = %q, missing client platform header", allow)
	}
}

func TestCORSAllowsStaticWebClient(t *testing.T) {
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil)}).Handler()
	req := httptest.NewRequest(http.MethodOptions, "https://server.example.com/v1/sessions", nil)
	req.Header.Set("Origin", "https://web.jaz.chat")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d", res.Code)
	}
	if allow := res.Header().Get("Access-Control-Allow-Origin"); allow != "https://web.jaz.chat" {
		t.Fatalf("Access-Control-Allow-Origin = %q", allow)
	}
	if allow := res.Header().Get("Access-Control-Allow-Private-Network"); allow != "true" {
		t.Fatalf("Access-Control-Allow-Private-Network = %q", allow)
	}
}

type blockingMCPRuntime struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingMCPRuntime) Refresh(ctx context.Context) {
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
	case <-ctx.Done():
	}
}

func (r *blockingMCPRuntime) Status(string) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *blockingMCPRuntime) Test(context.Context, mcpconfig.Server) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *blockingMCPRuntime) Authorize(context.Context, mcpconfig.Server, mcpconfig.AuthorizeOptions) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

type recordingMCPRuntime struct {
	options mcpconfig.AuthorizeOptions
}

func (r *recordingMCPRuntime) Refresh(context.Context) {}

func (r *recordingMCPRuntime) Status(string) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *recordingMCPRuntime) Test(context.Context, mcpconfig.Server) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *recordingMCPRuntime) Authorize(_ context.Context, _ mcpconfig.Server, opts mcpconfig.AuthorizeOptions) mcpconfig.ServerStatus {
	r.options = opts
	return mcpconfig.ServerStatus{
		Status:    "needs_auth",
		Error:     "Sign in required",
		AuthURL:   "https://auth.example.com/authorize",
		CheckedAt: time.Now().UTC(),
	}
}

func TestMCPAuthorizeBrowserUsesPublicCallbackAndReturnsAuthURL(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := store.CreateMCPServer(mcpconfig.ServerInput{
		Name:    "Notion",
		URL:     "https://mcp.notion.com/mcp",
		Enabled: true,
		OAuth:   mcpconfig.OAuthConfig{Issuer: "https://auth.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &recordingMCPRuntime{}
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, MCP: runtime}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/"+server.ID+"/authorize", nil)
	req.Header.Set(clientPlatformHeader, "browser")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "jaz.example.com")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("authorize status = %d, body = %s", res.Code, res.Body.String())
	}
	if runtime.options.RedirectURL != "https://jaz.example.com/v1/mcp/oauth/callback" ||
		!runtime.options.ReturnAuthURL ||
		runtime.options.OpenBrowser {
		t.Fatalf("authorize options = %#v", runtime.options)
	}
	var got struct {
		Status  string `json:"status"`
		AuthURL string `json:"auth_url"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "needs_auth" || got.AuthURL != "https://auth.example.com/authorize" {
		t.Fatalf("authorize response = %#v", got)
	}
}

func TestMCPAuthorizeUsesConfiguredPublicCallbackBeforeForwardedHost(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := store.CreateMCPServer(mcpconfig.ServerInput{
		Name:    "Notion",
		URL:     "https://mcp.notion.com/mcp",
		Enabled: true,
		OAuth:   mcpconfig.OAuthConfig{Issuer: "https://auth.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &recordingMCPRuntime{}
	handler := (&Server{
		ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		MCP:          runtime,
		ServerConfig: serverconfig.Config{Addr: ":5299", PublicURL: "https://public.example.com/app"},
	}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/"+server.ID+"/authorize", nil)
	req.Header.Set(clientPlatformHeader, "browser")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "spoofed.example.com")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("authorize status = %d, body = %s", res.Code, res.Body.String())
	}
	if runtime.options.RedirectURL != "https://public.example.com/v1/mcp/oauth/callback" {
		t.Fatalf("redirect url = %q", runtime.options.RedirectURL)
	}
}

func TestMCPServerSettingsRefreshDoesNotBlockResponse(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runtime := &blockingMCPRuntime{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(runtime.release)
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, MCP: runtime}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{
		"name":"Docs",
		"url":"https://mcp.example.com/mcp",
		"enabled":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", res.Code, res.Body.String())
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("response waited on refresh for %s", elapsed)
	}
	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("refresh was not scheduled")
	}
}

func TestMCPServerSettingsRejectInvalidURL(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{
		"name":"Docs",
		"url":"ftp://mcp.example.com/mcp",
		"enabled":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400, body = %s", res.Code, res.Body.String())
	}
}
