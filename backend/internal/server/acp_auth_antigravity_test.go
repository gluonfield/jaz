package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

func TestAntigravityDisconnectDoesNotWaitForSettingsProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell test harness is Unix-only")
	}
	home, bin := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	testexec.Write(t, filepath.Join(bin, "antigravity"), `#!/bin/sh
case "$1" in
acp)
    [ "$2" = "--disable-telemetry" ] || exit 2
    IFS= read -r init
    case "$init" in *'"method":"initialize"'*) ;; *) exit 3 ;; esac
    printf '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{"auth":{"logout":{}}},"authMethods":[{"id":"oauth-personal","name":"Google"}]}}\n'
    if IFS= read -r logout; then
        case "$logout" in *'"method":"logout"'*) ;; *) exit 8 ;; esac
        rm "$HOME/personal-sign-in" || exit 9
        (
            trap 'touch "$HOME/logout-worker-stopped"
                exit 0' TERM
            touch "$HOME/logout-worker-ready"
            while [ ! -f "$HOME/release-refresh" ]; do
                sleep 0.02
            done
        ) </dev/null >/dev/null 2>&1 &
        while [ ! -f "$HOME/logout-worker-ready" ]; do
            sleep 0.02
        done
        printf '{"jsonrpc":"2.0","id":2,"result":{}}\n'
    fi
    ;;
models)
    if [ -f "$HOME/personal-sign-in" ]; then
        printf x >> "$HOME/signed-in-probes"
        while [ ! -f "$HOME/release-old-probe" ]; do
            sleep 0.02
        done
        exit 0
    fi
    printf x >> "$HOME/probes"
    exit 1
    ;;
*) exit 7 ;;
esac
`, "")
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := disconnectTestServer(store, root, acp.AgentAntigravity)
	s.AuthKey = "test-key"
	cfg := s.AgentCatalog[acp.AgentAntigravity]
	cfg.ManagedAdapterArgs = []string{"acp", "--disable-telemetry"}
	s.AgentCatalog[acp.AgentAntigravity] = cfg
	if _, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{ACP: map[string]agentsettings.ACPAgentDefaults{
		acp.AgentAntigravity: {Enabled: true, Auth: acp.AgentAuthConfig{Mode: acp.AuthModeExistingCLI}},
	}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()

	mark := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(home, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	waitFor := func(name string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(home, name)); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("native CLI did not record %s", name)
	}

	mark("personal-sign-in")
	settingsReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/v1/settings/agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	settingsReq.Header.Set("Authorization", "Bearer test-key")
	oldProbeDone := make(chan error, 1)
	go func() {
		res, err := server.Client().Do(settingsReq)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK {
				err = fmt.Errorf("settings returned %s", res.Status)
			}
		}
		oldProbeDone <- err
	}()
	defer mark("release-old-probe")
	waitFor("signed-in-probes")

	disconnectReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v1/acp/agents/antigravity/auth/disconnect", nil)
	if err != nil {
		t.Fatal(err)
	}
	disconnectReq.Header.Set("Authorization", "Bearer test-key")
	type disconnectResponse struct {
		Result acpDisconnectResponse
		Err    error
	}
	disconnected := make(chan disconnectResponse, 1)
	go func() {
		var result disconnectResponse
		res, err := server.Client().Do(disconnectReq)
		if err == nil {
			defer res.Body.Close()
			if res.StatusCode == http.StatusOK {
				err = json.NewDecoder(res.Body).Decode(&result.Result)
			} else {
				err = fmt.Errorf("disconnect returned %s", res.Status)
			}
		}
		result.Err = err
		disconnected <- result
	}()
	defer mark("release-refresh")

	select {
	case response := <-disconnected:
		result := response.Result
		if response.Err != nil || result.Settings.Enabled || result.Settings.Auth.Mode != acp.AuthModeAuto || result.Auth.Authenticated || !result.Auth.LoginCommandAvailable {
			t.Fatalf("expected disabled and signed-out settings, got %+v", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect waited for a settings probe")
	}
	if _, err := os.Stat(filepath.Join(home, "probes")); !os.IsNotExist(err) {
		t.Fatalf("disconnect probed signed-out authentication: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "logout-worker-stopped")); err != nil {
		t.Fatalf("disconnect did not clean up the native process group: %v", err)
	}
	mark("release-old-probe")
	select {
	case err := <-oldProbeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("settings request started before disconnect did not return")
	}
}
