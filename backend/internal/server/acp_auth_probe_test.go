package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	modelcapabilitiesapi "github.com/wins/jaz/backend/internal/httpapi/modelcapabilities"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

func TestAgentSettingsDoesNotBlockOnSlowAntigravityProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell test harness is Unix-only")
	}
	clearAgentEnv(t)
	home, bin := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	testexec.Write(t, filepath.Join(bin, "antigravity"), `#!/bin/sh
if [ "$1" != "models" ]; then exit 2; fi
printf x >> "$HOME/agy-probes"
touch "$HOME/agy-started"
while [ ! -f "$HOME/agy-release" ]; do
	sleep 0.02
done
printf 'gemini-4-neon-high\tGemini 4 Neon (High)\n'
`, "")

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	codexExe := testexec.Write(t, filepath.Join(root, "codex-acp"), "", "")
	codexHome := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	writeCodexOAuth(t, codexHome)
	t.Setenv("CODEX_HOME", codexHome)

	catalog := acp.AgentCatalog{
		acp.AgentCodex:       {Command: codexExe, Model: "gpt-5.5"},
		acp.AgentAntigravity: {ManagedAdapter: acp.AgentAntigravity},
	}
	acpManager := acp.NewManager(nil, acp.Config{
		Root:         root,
		Agents:       catalog,
		ModelCatalog: modelcatalog.NewService(nil),
	}, nil)
	defer acpManager.Close()

	srv := &Server{
		ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		Root:         root,
		AgentCatalog: catalog,
		ACP:          acpManager,
		Routes: Routes{
			{Pattern: "GET /v1/acp/agents/{agent}/models", Handler: modelcapabilitiesapi.NewAgentModelsHandler(acpManager)},
		},
	}
	handler := srv.Handler()
	t.Cleanup(func() {
		_ = os.WriteFile(filepath.Join(home, "agy-release"), nil, 0o600)
	})

	getSettings := func() agentSettingsResponse {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil))
		if res.Code != http.StatusOK {
			t.Fatalf("GET /v1/settings/agents status = %d, body = %s", res.Code, res.Body.String())
		}
		var body agentSettingsResponse
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	// First request returns immediately while Antigravity is still blocked.
	start := time.Now()
	first := getSettings()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("GET /v1/settings/agents blocked on slow Antigravity probe (%v)", elapsed)
	}
	if !first.ACPAuth[acp.AgentCodex].Ready || first.ACPAuth[acp.AgentCodex].Checking {
		t.Fatalf("fast codex agent should be ready immediately, got %+v", first.ACPAuth[acp.AgentCodex])
	}
	if !first.ACPAuth[acp.AgentAntigravity].Checking || first.ACPAuth[acp.AgentAntigravity].Ready {
		t.Fatalf("slow antigravity agent should report checking=true while probing, got %+v", first.ACPAuth[acp.AgentAntigravity])
	}

	// Wait until the single background probe is actually running, then hit settings again.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(home, "agy-started")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	second := getSettings()
	if !second.ACPAuth[acp.AgentAntigravity].Checking {
		t.Fatalf("expected second poll while blocked to still report checking=true, got %+v", second.ACPAuth[acp.AgentAntigravity])
	}
	if probes, err := os.ReadFile(filepath.Join(home, "agy-probes")); err != nil || len(probes) != 1 {
		t.Fatalf("expected 1 coalesced background probe, got %q, %v", probes, err)
	}

	// Release the slow probe and confirm Antigravity settles to ready without re-running `agy models` for model discovery.
	if err := os.WriteFile(filepath.Join(home, "agy-release"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var settled agentSettingsResponse
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		settled = getSettings()
		if !settled.ACPAuth[acp.AgentAntigravity].Checking {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if settled.ACPAuth[acp.AgentAntigravity].Checking || !settled.ACPAuth[acp.AgentAntigravity].Authenticated || !settled.ACPAuth[acp.AgentAntigravity].Ready {
		t.Fatalf("antigravity did not settle to ready: %+v", settled.ACPAuth[acp.AgentAntigravity])
	}

	modelsRes := httptest.NewRecorder()
	handler.ServeHTTP(modelsRes, httptest.NewRequest(http.MethodGet, "/v1/acp/agents/antigravity/models", nil))
	if modelsRes.Code != http.StatusOK {
		t.Fatalf("GET /v1/acp/agents/antigravity/models status = %d, body = %s", modelsRes.Code, modelsRes.Body.String())
	}
	var catalogBody acp.AgentModelCatalog
	if err := json.Unmarshal(modelsRes.Body.Bytes(), &catalogBody); err != nil {
		t.Fatal(err)
	}
	if len(catalogBody.Models) != 1 || catalogBody.Models[0].Value != "gemini-4-neon" {
		t.Fatalf("unexpected antigravity models: %+v", catalogBody)
	}
	if probes, err := os.ReadFile(filepath.Join(home, "agy-probes")); err != nil || len(probes) != 1 {
		t.Fatalf("model discovery spawned a duplicate agy models process: %q, %v", probes, err)
	}
}

func TestOnboardingDoesNotBlockOnSlowAntigravityProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell test harness is Unix-only")
	}
	clearAgentEnv(t)
	home, bin := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	testexec.Write(t, filepath.Join(bin, "antigravity"), `#!/bin/sh
if [ "$1" != "models" ]; then exit 2; fi
printf x >> "$HOME/agy-probes"
while [ ! -f "$HOME/agy-release" ]; do
	sleep 0.02
done
exit 0
`, "")
	t.Cleanup(func() {
		_ = os.WriteFile(filepath.Join(home, "agy-release"), nil, 0o600)
	})

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	codexExe := testexec.Write(t, filepath.Join(root, "codex-acp"), "", "")
	codexHome := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	writeCodexOAuth(t, codexHome)
	t.Setenv("CODEX_HOME", codexHome)

	handler := (&Server{
		ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		Root:         root,
		AgentCatalog: acp.AgentCatalog{
			acp.AgentCodex:       {Command: codexExe, Model: "gpt-5.5"},
			acp.AgentAntigravity: {ManagedAdapter: acp.AgentAntigravity},
		},
	}).Handler()

	start := time.Now()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/onboarding", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/onboarding status = %d, body = %s", res.Code, res.Body.String())
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("GET /v1/onboarding blocked on slow Antigravity probe (%v)", elapsed)
	}
	var got onboardingResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Settings.ACPAuth[acp.AgentCodex].Ready || !got.Settings.ACPAuth[acp.AgentAntigravity].Checking {
		t.Fatalf("unexpected onboarding settings auth: %+v", got.Settings.ACPAuth)
	}
}
