package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/acpadapter"
	"github.com/wins/jaz/backend/internal/managedtool"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/onboardingstate"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

type fakeACPAdapterStatusReader struct {
	status     acpadapter.Status
	prepared   *int
	prepareErr error
}

func (r fakeACPAdapterStatusReader) Status(string) acpadapter.Status {
	return r.status
}

func (r fakeACPAdapterStatusReader) Prepare(context.Context, string) error {
	if r.prepared != nil {
		*r.prepared = *r.prepared + 1
	}
	return r.prepareErr
}

type fakeManagedToolStatusReader struct {
	status     managedtool.Status
	prepared   *int
	prepareErr error
}

func (r fakeManagedToolStatusReader) Status(string) managedtool.Status {
	return r.status
}

func (r fakeManagedToolStatusReader) Prepare(context.Context, string) error {
	if r.prepared != nil {
		*r.prepared = *r.prepared + 1
	}
	return r.prepareErr
}

func TestOnboardingAPIProbesAgentsAndSavesProviderKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exe := testexec.Write(t, filepath.Join(root, "codex-acp"), "", "")
	codexHome := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store: store,
		Root:  root,
		AgentCatalog: acp.AgentCatalog{
			"codex":  {Command: exe, Model: "gpt-5.5"},
			"claude": {Command: "definitely-missing-jaz-agent", Model: "default"},
		},
	}).Handler()

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/v1/onboarding", nil))
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
	var got struct {
		Completed bool `json:"completed"`
		ACP       []struct {
			Agent     string `json:"agent"`
			Available bool   `json:"available"`
		} `json:"acp"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Completed || !agentProbeAvailable(got.ACP, "codex") || agentProbeAvailable(got.ACP, "claude") {
		t.Fatalf("unexpected onboarding status: %#v", got)
	}

	postRes := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "/v1/onboarding", jsonReader(t, map[string]any{
		"settings": map[string]any{
			"acp": map[string]any{
				"codex": map[string]any{
					"enabled":          true,
					"command":          exe,
					"model":            "gpt-5.5",
					"reasoning_effort": "medium",
				},
				"claude": map[string]any{
					"enabled":          false,
					"command":          "definitely-missing-jaz-agent",
					"model":            "default",
					"reasoning_effort": "medium",
				},
			},
		},
		"provider_keys": map[string]any{"openrouter": "runtime-key"},
		"acp_keys":      map[string]any{"codex": "codex-key"},
		"completed":     true,
	}))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(postRes, postReq)
	if postRes.Code != http.StatusOK {
		t.Fatalf("post status = %d, body = %s", postRes.Code, postRes.Body.String())
	}
	assertRuntimeKeySaved(t, root)
	assertACPKeySaved(t, root)
	assertOnboardingStateSaved(t, root)
	memorySettings, err := agentsettings.LoadMemorySettings(store)
	if err != nil {
		t.Fatal(err)
	}
	if memorySettings.Agent != acp.AgentCodex {
		t.Fatalf("memory agent = %q", memorySettings.Agent)
	}
	var saved struct {
		Completed bool `json:"completed"`
		Settings  struct {
			Providers []struct {
				ID         string `json:"id"`
				Configured bool   `json:"configured"`
			} `json:"providers"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(postRes.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Completed || !modelProviderConfigured(saved.Settings.Providers, "openrouter") {
		t.Fatalf("unexpected saved onboarding status: %#v", saved)
	}
}

func TestOnboardingSavesExplicitMemorySettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
			"codex": {Command: exe, Model: "gpt-5.5"},
		},
	}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding", jsonReader(t, map[string]any{
		"settings": map[string]any{
			"acp": map[string]any{
				"codex": map[string]any{
					"enabled": true,
					"command": exe,
					"model":   "gpt-5.5",
				},
			},
		},
		"acp_keys": map[string]any{"codex": "codex-key"},
		"memory": map[string]any{
			"enabled": false,
			"agent":   "codex",
		},
		"completed": true,
	}))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	memorySettings, err := agentsettings.LoadMemorySettings(store)
	if err != nil {
		t.Fatal(err)
	}
	if memorySettings.Enabled || memorySettings.Agent != acp.AgentCodex {
		t.Fatalf("memory settings = %#v", memorySettings)
	}
}

func TestOnboardingRejectsEnabledMemoryWithoutAgent(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root, AgentCatalog: acp.AgentCatalog{}}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding", strings.NewReader(`{
		"memory":{"enabled":true},
		"completed":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "memory agent is required") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestOnboardingAllowsAuthenticatedRemoteProviderKeySetup(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root, AuthKey: "secret"}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding", strings.NewReader(`{
		"provider_keys":{"openrouter":"runtime-key"}
	}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	assertRuntimeKeySaved(t, root)
}

func TestOnboardingUsesACPReadiness(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)
	shell := testexec.Write(t, filepath.Join(root, "shell"), "", "")
	t.Setenv("SHELL", shell)
	t.Setenv("CLAUDE_CODE_EXECUTABLE", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "token")
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exe := testexec.Write(t, filepath.Join(root, "claude-acp"), "", "")
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store: store,
		Root:  root,
		AgentCatalog: acp.AgentCatalog{
			"claude": {Command: exe, Model: "default"},
		},
	}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/onboarding", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACP []struct {
			Agent         string `json:"agent"`
			Authenticated bool   `json:"authenticated"`
			Available     bool   `json:"available"`
			Reason        string `json:"reason"`
		} `json:"acp"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ACP) != 1 || got.ACP[0].Agent != "claude" || !got.ACP[0].Authenticated || got.ACP[0].Available || !strings.Contains(got.ACP[0].Reason, "Claude Code executable") {
		t.Fatalf("unexpected claude probe: %#v", got.ACP)
	}
}

func TestOnboardingTreatsAuthenticatedManagedCodexAsAvailable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", root)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("JAZ_ACP_CODEX_API_KEY", "")
	codexHome := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{ACP: map[string]agentsettings.ACPAgentDefaults{
		"codex": {Command: "/stale/codex-acp --stdio", Model: "gpt-5.5"},
	}}); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store:       store,
		Root:        root,
		ACPAdapters: fakeACPAdapterStatusReader{status: acpadapter.Status{Adapter: "codex", Version: "test-version", State: acpadapter.StateReady}},
		AgentCatalog: acp.AgentCatalog{
			"codex": {ManagedAdapter: "codex", Model: "gpt-5.5"},
		},
	}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/onboarding", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACP []struct {
			Agent         string `json:"agent"`
			Command       string `json:"command"`
			Installed     bool   `json:"installed"`
			Authenticated bool   `json:"authenticated"`
			Available     bool   `json:"available"`
			Reason        string `json:"reason"`
		} `json:"acp"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ACP) != 1 || got.ACP[0].Agent != "codex" ||
		got.ACP[0].Command != "" ||
		!got.ACP[0].Installed ||
		!got.ACP[0].Authenticated ||
		!got.ACP[0].Available ||
		got.ACP[0].Reason != "" {
		t.Fatalf("unexpected codex probe: %#v", got.ACP)
	}
}

func TestOnboardingWaitsForManagedCodexDownload(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", root)
	codexHome := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store:       store,
		Root:        root,
		ACPAdapters: fakeACPAdapterStatusReader{status: acpadapter.Status{Adapter: "codex", Version: "test-version", State: acpadapter.StateDownloading, Message: "Downloading Codex adapter"}},
		AgentCatalog: acp.AgentCatalog{
			"codex": {ManagedAdapter: "codex", Model: "gpt-5.5"},
		},
	}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/onboarding", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACP []struct {
			Agent          string `json:"agent"`
			Authenticated  bool   `json:"authenticated"`
			Available      bool   `json:"available"`
			Reason         string `json:"reason"`
			ManagedAdapter struct {
				State string `json:"state"`
			} `json:"managed_adapter"`
		} `json:"acp"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ACP) != 1 || got.ACP[0].Agent != "codex" ||
		!got.ACP[0].Authenticated ||
		got.ACP[0].Available ||
		got.ACP[0].ManagedAdapter.State != acpadapter.StateDownloading ||
		!strings.Contains(got.ACP[0].Reason, "Downloading Codex adapter") {
		t.Fatalf("unexpected codex probe: %#v", got.ACP)
	}
}

func TestOnboardingWaitsForManagedAntigravityCLI(t *testing.T) {
	root := t.TempDir()
	clearAgentEnv(t)
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{
		ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		Root:         root,
		ACPAdapters:  fakeACPAdapterStatusReader{status: acpadapter.Status{Adapter: "antigravity", State: acpadapter.StateReady}},
		ManagedTools: fakeManagedToolStatusReader{status: managedtool.Status{Tool: managedtool.AntigravityCLI, State: managedtool.StateMissing, Message: "Antigravity CLI is not downloaded yet"}},
		AgentCatalog: acp.AgentCatalog{
			acp.AgentAntigravity: {ManagedAdapter: "antigravity"},
		},
	}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/onboarding", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACP []struct {
			Agent       string `json:"agent"`
			Installed   bool   `json:"installed"`
			Available   bool   `json:"available"`
			Reason      string `json:"reason"`
			ManagedTool struct {
				State string `json:"state"`
			} `json:"managed_tool"`
		} `json:"acp"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ACP) != 1 || got.ACP[0].Agent != acp.AgentAntigravity ||
		got.ACP[0].Installed ||
		got.ACP[0].Available ||
		got.ACP[0].ManagedTool.State != managedtool.StateMissing ||
		!strings.Contains(got.ACP[0].Reason, "Antigravity CLI") {
		t.Fatalf("unexpected antigravity probe: %#v", got.ACP)
	}
}

func TestPrepareACPAgentInstallsAntigravityAdapterAndCLI(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	adapterPrepared := 0
	toolPrepared := 0
	handler := (&Server{
		ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		Root:         root,
		ACPAdapters:  fakeACPAdapterStatusReader{status: acpadapter.Status{Adapter: "antigravity", State: acpadapter.StateMissing}, prepared: &adapterPrepared},
		ManagedTools: fakeManagedToolStatusReader{status: managedtool.Status{Tool: managedtool.AntigravityCLI, State: managedtool.StateMissing}, prepared: &toolPrepared},
		AgentCatalog: acp.AgentCatalog{
			acp.AgentAntigravity: {ManagedAdapter: "antigravity"},
		},
	}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/acp/agents/antigravity/prepare", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if adapterPrepared != 1 || toolPrepared != 1 {
		t.Fatalf("prepared adapter=%d tool=%d, want both once", adapterPrepared, toolPrepared)
	}
}

func TestOnboardingIgnoresClaudeSettingsOnlyConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{
		"CLAUDE_CONFIG_DIR", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY", "JAZ_ACP_CLAUDE_API_KEY",
	} {
		t.Setenv(key, "")
	}
	globalDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(globalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, ".claude.json"), []byte(`{"hasCompletedOnboarding":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exe := testexec.Write(t, filepath.Join(root, "claude-acp"), "", "")
	t.Setenv("CLAUDE_CODE_EXECUTABLE", exe)
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store: store,
		Root:  root,
		AgentCatalog: acp.AgentCatalog{
			"claude": {Command: exe, Model: "default"},
		},
	}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/onboarding", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACP []struct {
			Agent         string `json:"agent"`
			Authenticated bool   `json:"authenticated"`
			Available     bool   `json:"available"`
			Reason        string `json:"reason"`
		} `json:"acp"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ACP) != 1 || got.ACP[0].Agent != "claude" || got.ACP[0].Authenticated || got.ACP[0].Available || !strings.Contains(got.ACP[0].Reason, "Claude login") {
		t.Fatalf("unexpected claude probe: %#v", got.ACP)
	}
}

func TestAppBundleInstalledIn(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Claude.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !appBundleInstalledIn([]string{root}, "Claude.app") {
		t.Fatal("expected Claude.app to be detected")
	}
	if appBundleInstalledIn([]string{root}, "Missing.app") {
		t.Fatal("unexpected app bundle detection")
	}
}

func TestOnboardingRejectsUnauthenticatedRemoteProviderKeySetup(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding", strings.NewReader(`{
		"provider_keys":{"openrouter":"runtime-key"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(runtimeenv.Path(root)); !os.IsNotExist(err) {
		t.Fatalf("runtime env should not be written, err = %v", err)
	}
}

func assertOnboardingStateSaved(t *testing.T, root string) {
	t.Helper()
	state, found, err := onboardingstate.Load(onboardingstate.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if !found || !state.Completed {
		t.Fatalf("onboarding state = %#v, found = %v", state, found)
	}
	info, err := os.Stat(onboardingstate.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("onboarding state permissions = %v", info.Mode().Perm())
	}
}

func clearAgentEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	for _, key := range []string{
		"CODEX_HOME",
		"OPENAI_API_KEY",
		"OPENAI_APIKEY",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_APIKEY",
		"CLAUDE_CONFIG_DIR",
		"CLAUDE_CODE_OAUTH_TOKEN",
		"GEMINI_API_KEY",
		"GEMINI_APIKEY",
		"JAZ_ACP_CODEX_API_KEY",
		"JAZ_ACP_CLAUDE_API_KEY",
		"JAZ_ACP_GROK_API_KEY",
		"JAZ_ACP_OPENCODE_API_KEY",
	} {
		t.Setenv(key, "")
	}
}

func assertRuntimeKeySaved(t *testing.T, root string) {
	t.Helper()
	env, err := os.ReadFile(runtimeenv.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), `OPENROUTER_API_KEY="runtime-key"`) {
		t.Fatalf("runtime env = %s", env)
	}
	info, err := os.Stat(runtimeenv.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("env permissions = %v", info.Mode().Perm())
	}
}

func assertACPKeySaved(t *testing.T, root string) {
	t.Helper()
	env, err := os.ReadFile(runtimeenv.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), `JAZ_ACP_CODEX_API_KEY="codex-key"`) {
		t.Fatalf("runtime env = %s", env)
	}
}

func agentProbeAvailable(probes []struct {
	Agent     string `json:"agent"`
	Available bool   `json:"available"`
}, agent string) bool {
	for _, probe := range probes {
		if probe.Agent == agent {
			return probe.Available
		}
	}
	return false
}

func modelProviderConfigured(providers []struct {
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
