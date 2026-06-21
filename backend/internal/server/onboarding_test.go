package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/onboardingstate"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

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
	handler := (&Server{
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
	handler := (&Server{
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
	handler := (&Server{Store: store, Root: root, AgentCatalog: acp.AgentCatalog{}}).Handler()

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
	handler := (&Server{Store: store, Root: root, AuthKey: "secret"}).Handler()

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
	handler := (&Server{
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
	handler := (&Server{
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
	handler := (&Server{Store: store, Root: root}).Handler()

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
