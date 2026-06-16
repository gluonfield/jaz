package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

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
	handler := (&Server{Store: store}).Handler()

	getReq := httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil)
	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
	var got struct {
		Native struct {
			ModelProvider string `json:"model_provider"`
			Model         string `json:"model"`
		} `json:"native"`
		Providers []struct {
			ID      string `json:"id"`
			BaseURL string `json:"base_url"`
		} `json:"providers"`
		Agents []string `json:"agents"`
		ACP    map[string]struct {
			Enabled         bool   `json:"enabled"`
			Command         string `json:"command"`
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
		} `json:"acp"`
		ACPOptions map[string]struct {
			ReasoningEfforts []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"reasoning_efforts"`
		} `json:"acp_options"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Native.ModelProvider != "openrouter" || got.Native.Model != "openai/gpt-5.4-mini" || strings.Join(got.Agents, ",") != "claude,codex,grok" {
		t.Fatalf("unexpected seeded settings %#v", got)
	}
	if !hasNativeProvider(got.Providers, "openai", "https://api.openai.com/v1") ||
		!hasNativeProvider(got.Providers, "openrouter", "https://openrouter.ai/api/v1") {
		t.Fatalf("providers = %#v", got.Providers)
	}
	if !got.ACP["codex"].Enabled ||
		got.ACP["codex"].Command != `npx -y @jazchat/codex-acp@0.16.1 -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'` ||
		got.ACP["codex"].Model != "gpt-5.5" {
		t.Fatalf("unexpected codex defaults %#v", got.ACP["codex"])
	}
	if !got.ACP["grok"].Enabled ||
		got.ACP["grok"].Command != `grok --no-auto-update agent --no-leader --always-approve stdio` ||
		got.ACP["grok"].Model != "grok-build" {
		t.Fatalf("unexpected grok defaults %#v", got.ACP["grok"])
	}
	if !hasReasoningEffort(got.ACPOptions["claude"].ReasoningEfforts, "ultracode") ||
		hasReasoningEffort(got.ACPOptions["codex"].ReasoningEfforts, "ultracode") {
		t.Fatalf("unexpected acp options %#v", got.ACPOptions)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"native":{"model_provider":"openrouter","model":"openai/gpt-5.4-mini","reasoning_effort":"medium"},
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

func TestAgentSettingsSavesNativeProviderKey(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store, Root: root}).Handler()

	putReq := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"native":{"model_provider":"openrouter","model":"openai/gpt-5.4-mini","reasoning_effort":"medium"},
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

func TestAgentSettingsRejectsInvalidSettingsBeforeSavingACPKeys(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"native":{"model_provider":"openrouter","model":"openai/gpt-5.4-mini","reasoning_effort":"medium"},
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

func hasNativeProvider(providers []struct {
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

func TestAgentSettingsAPIRoundTripsConfiguredACPAgent(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog := acp.MergeAgents(acp.BuiltinAgents(), map[string]acp.AgentConfig{
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
	if strings.Join(got.Agents, ",") != "claude,codex,grok,local_helper" {
		t.Fatalf("agents = %#v", got.Agents)
	}
	if got.ACP["local_helper"].Command != "/opt/jaz/local-helper --stdio" || got.ACP["local_helper"].Model != "helper-model" {
		t.Fatalf("custom agent not seeded: %#v", got.ACP["local_helper"])
	}

	putReq := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"native":{"model_provider":"openrouter","model":"openai/gpt-5.4-mini","reasoning_effort":"medium"},
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
		"native":{"model_provider":"openrouter","model":"openai/gpt-5.4-mini"},
		"acp":{"missing":{"enabled":true}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "unknown acp agent") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAgentSettingsRejectUnknownNativeProvider(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"native":{"model_provider":"missing","model":"test-model"},
		"acp":{"codex":{"enabled":false}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "unknown native provider") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAgentSettingsRejectMockNativeProvider(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{
		"native":{"model_provider":"mock","model":"test-model"},
		"acp":{"codex":{"enabled":false}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "unknown native provider") {
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
		"native":{"model_provider":"openrouter","model":"openai/gpt-5.4-mini"},
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
