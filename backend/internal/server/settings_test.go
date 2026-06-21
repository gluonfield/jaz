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
	"github.com/wins/jaz/backend/internal/jazagent"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

func testACPAgentCatalog(extra map[string]acp.AgentConfig) acp.AgentCatalog {
	return acp.MergeAgents(acp.MergeAgents(acp.BuiltinAgents(), jazagent.ACPAgentCatalog()), extra)
}

func TestMCPServerSettingsAPI(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store}).Handler()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{
		"name":"Docs",
		"url":"https://mcp.example.com/mcp",
		"enabled":true,
		"bearer_token_env_var":"DOCS_TOKEN",
		"headers":[{"name":"X-Team","value":"platform"}],
		"env_headers":[{"name":"X-Secret","env_var":"DOCS_SECRET"}]
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
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "Docs" || created.URL != "https://mcp.example.com/mcp" ||
		!created.Enabled || created.BearerTokenEnvVar != "DOCS_TOKEN" {
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
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
			EnvHeaders []struct {
				Name   string `json:"name"`
				EnvVar string `json:"env_var"`
			} `json:"env_headers"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Servers) != 1 || listed.Servers[0].ID != created.ID ||
		len(listed.Servers[0].Headers) != 1 || listed.Servers[0].Headers[0].Value != "platform" ||
		len(listed.Servers[0].EnvHeaders) != 1 || listed.Servers[0].EnvHeaders[0].EnvVar != "DOCS_SECRET" {
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
	handler := (&Server{Store: store, AgentCatalog: testACPAgentCatalog(nil)}).Handler()

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
			Command         string `json:"command"`
			ModelProvider   string `json:"model_provider"`
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
		} `json:"acp"`
		ACPOptions map[string]struct {
			ReasoningEfforts []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"reasoning_efforts"`
			Local            bool     `json:"local"`
			ProviderMode     string   `json:"provider_mode"`
			ModelProviderIDs []string `json:"model_provider_ids"`
			RequiresCommand  bool     `json:"requires_command"`
			SupportsAuth     bool     `json:"supports_auth"`
		} `json:"acp_options"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Agents, ",") != "claude,codex,grok,jaz,opencode" {
		t.Fatalf("unexpected seeded settings %#v", got)
	}
	if !hasModelProvider(got.Providers, "openai", "https://api.openai.com/v1") ||
		!hasModelProvider(got.Providers, "openrouter", "https://openrouter.ai/api/v1") {
		t.Fatalf("providers = %#v", got.Providers)
	}
	if got.ACP["codex"].Enabled ||
		got.ACP["codex"].Command != `npx -y @jazchat/codex-acp@0.16.1 -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"' -c features.tool_search_always_defer_mcp_tools=true` ||
		got.ACP["codex"].Model != "gpt-5.5" {
		t.Fatalf("unexpected codex defaults %#v", got.ACP["codex"])
	}
	if got.ACP["grok"].Enabled ||
		got.ACP["grok"].Command != `grok --no-auto-update agent --no-leader --always-approve stdio` ||
		got.ACP["grok"].Model != "grok-build" {
		t.Fatalf("unexpected grok defaults %#v", got.ACP["grok"])
	}
	if !got.ACP["jaz"].Enabled ||
		got.ACP["jaz"].Command != "" ||
		got.ACP["jaz"].ModelProvider != "openrouter" ||
		got.ACP["jaz"].Model != "openai/gpt-5.4-mini" {
		t.Fatalf("unexpected jaz defaults %#v", got.ACP["jaz"])
	}
	if got.ACP["opencode"].Enabled ||
		got.ACP["opencode"].Command != `npx -y opencode-ai@1.17.7 acp` ||
		got.ACP["opencode"].ModelProvider != "openrouter" ||
		got.ACP["opencode"].Model != "openai/gpt-5.4-mini" {
		t.Fatalf("unexpected opencode defaults %#v", got.ACP["opencode"])
	}
	if !hasReasoningEffort(got.ACPOptions["claude"].ReasoningEfforts, "ultracode") ||
		hasReasoningEffort(got.ACPOptions["codex"].ReasoningEfforts, "ultracode") {
		t.Fatalf("unexpected acp options %#v", got.ACPOptions)
	}
	if !got.ACPOptions["jaz"].Local ||
		got.ACPOptions["jaz"].ProviderMode != acp.AgentProviderModeAgentDefaults ||
		!hasString(got.ACPOptions["jaz"].ModelProviderIDs, "openrouter") ||
		!hasString(got.ACPOptions["jaz"].ModelProviderIDs, "openai") ||
		got.ACPOptions["jaz"].RequiresCommand ||
		got.ACPOptions["jaz"].SupportsAuth {
		t.Fatalf("unexpected jaz capabilities %#v", got.ACPOptions["jaz"])
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
	for _, flag := range []string{"local", "requires_command", "supports_auth"} {
		if _, ok := rawOptions.ACPOptions["jaz"][flag]; !ok {
			t.Fatalf("acp_options.jaz must emit %q, body = %s", flag, getRes.Body.String())
		}
	}
	if got.ACPOptions["opencode"].ProviderMode != acp.AgentProviderModeAgentDefaults ||
		!hasString(got.ACPOptions["opencode"].ModelProviderIDs, "openrouter") ||
		!hasString(got.ACPOptions["opencode"].ModelProviderIDs, "openai") {
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
		!strings.Contains(string(loaded.Value), `/opt/jaz/codex-acp`) ||
		!strings.Contains(string(loaded.Value), `"reasoning_effort":"high"`) {
		t.Fatalf("stored settings = %s", loaded.Value)
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
	handler := (&Server{Store: store, Root: root}).Handler()

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
	handler := (&Server{
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
	handler := (&Server{
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
	handler := (&Server{
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

	(&Server{Store: store, Root: root}).Handler().ServeHTTP(res, req)

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

func TestAgentSettingsAPIIncludesCustomOpenCodeProvider(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog := testACPAgentCatalog(acp.AgentCatalog{
		acp.AgentOpenCode: {
			Command:       "opencode",
			ModelProvider: "internal",
			Model:         "chat",
		},
	})
	handler := (&Server{
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
		!custom.OpenCode || !custom.OpenAICompatible || !custom.Configured {
		t.Fatalf("custom provider not exposed correctly: %#v", got.Providers)
	}
	if options := got.ACPOptions["opencode"]; options.ProviderMode != acp.AgentProviderModeAgentDefaults ||
		!hasString(options.ModelProviderIDs, "internal") {
		t.Fatalf("opencode capabilities lost: %#v", options)
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
		"local_helper": {
			Command:         "/opt/jaz/local-helper",
			Args:            []string{"--stdio"},
			Model:           "helper-model",
			ReasoningEffort: "low",
		},
	})
	handler := (&Server{Store: store, AgentCatalog: catalog}).Handler()

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
			Command string `json:"command"`
			Model   string `json:"model"`
		} `json:"acp"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Agents, ",") != "claude,codex,grok,jaz,local_helper,opencode" {
		t.Fatalf("agents = %#v", got.Agents)
	}
	if got.ACP["local_helper"].Command != "/opt/jaz/local-helper --stdio" || got.ACP["local_helper"].Model != "helper-model" {
		t.Fatalf("custom agent not seeded: %#v", got.ACP["local_helper"])
	}

	putReq := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"acp":{
			"codex":{"enabled":false,"command":"codex-acp","model":"gpt-5.5","reasoning_effort":"medium"},
			"claude":{"enabled":false,"command":"npx -y @agentclientprotocol/claude-agent-acp@0.43.0","model":"default","reasoning_effort":"medium"},
			"local_helper":{"enabled":true,"command":"/opt/jaz/local-helper --stdio","model":"helper-model","reasoning_effort":"low"}
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

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "unknown acp agent") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAgentSettingsRejectEnabledACPWithoutCommand(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"acp":{
			"codex":{"enabled":true,"command":""},
			"claude":{"enabled":false,"command":""}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "command is required") {
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
	handler := (&Server{Store: store}).Handler()

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

func (r *blockingMCPRuntime) Authorize(context.Context, mcpconfig.Server) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
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
	handler := (&Server{Store: store, MCP: runtime}).Handler()

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
	handler := (&Server{Store: store}).Handler()

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
