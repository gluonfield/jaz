package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/acp"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/terminal"
	"github.com/wins/jaz/backend/internal/testexec"
)

func TestAntigravityCloudSignInUsesNativeTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native terminal is Unix-only")
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
    version=1
    if [ -f "$HOME/fail-discovery" ]; then version=99; fi
    printf '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":%s,"agentCapabilities":{"auth":{"logout":{}}},"authMethods":[{"id":"oauth-business","name":"Cloud"},{"id":"oauth-personal","name":"Google"}]}}\n' "$version"
    if IFS= read -r logout; then
        case "$logout" in *'"method":"logout"'*) ;; *) exit 8 ;; esac
        rm "$HOME/personal-sign-in" || exit 9
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
    if [ ! -f "$HOME/refreshing" ]; then
        touch "$HOME/refreshing"
        while [ ! -f "$HOME/release-refresh" ]; do
            sleep 0.02
        done
        exit 1
    fi
    test -f "$HOME/cloud-sign-in" || exit 1
    touch "$HOME/verifying"
    while [ ! -f "$HOME/release-verification" ]; do sleep 0.02; done
    ;;
"")
    test -n "$SSH_CONNECTION" || exit 4
    test -t 0 || exit 5
    echo 'Use a Google Cloud project'
    IFS= read -r project
    [ "$project" = test-project ] || exit 6
    touch "$HOME/cloud-sign-in"
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
	path := "/v1/acp/agents/antigravity/auth"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path+"/methods", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-key")
	failDiscovery := filepath.Join(home, "fail-discovery")
	if err := os.WriteFile(failDiscovery, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	failure, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = failure.Body.Close()
	if failure.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("failed capability probe status = %d", failure.StatusCode)
	}
	if err := os.Remove(failDiscovery); err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var choices []string
	if err := json.NewDecoder(response.Body).Decode(&choices); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(choices, []string{acp.AntigravityGoogleAccount, acp.AntigravityCloudProject}) {
		t.Fatalf("offered sign-in methods: %v", choices)
	}
	open := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL, path+"/cloud-terminal"), request.Header)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	next := func(conn *websocket.Conn) terminal.Message {
		t.Helper()
		var event terminal.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatal(err)
		}
		return event
	}
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
	disconnect, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+path+"/disconnect", nil)
	if err != nil {
		t.Fatal(err)
	}
	disconnect.Header = request.Header.Clone()
	type disconnectResponse struct {
		Settings agentSettingsResponse
		Err      error
	}
	disconnected := make(chan disconnectResponse, 1)
	mark("personal-sign-in")
	oldSettings, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/v1/settings/agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	oldSettings.Header = request.Header.Clone()
	oldProbeDone := make(chan error, 1)
	go func() {
		res, err := server.Client().Do(oldSettings)
		if err == nil {
			err = res.Body.Close()
		}
		oldProbeDone <- err
	}()
	defer mark("release-old-probe")
	waitFor("signed-in-probes")
	go func() {
		var result disconnectResponse
		res, err := server.Client().Do(disconnect)
		if err == nil {
			defer res.Body.Close()
			if res.StatusCode == http.StatusOK {
				err = json.NewDecoder(res.Body).Decode(&result.Settings)
			} else {
				err = fmt.Errorf("disconnect returned %s", res.Status)
			}
		}
		result.Err = err
		disconnected <- result
	}()
	defer mark("release-refresh")
	waitFor("refreshing")
	if probes, err := os.ReadFile(filepath.Join(home, "signed-in-probes")); err != nil || len(probes) != 1 {
		t.Fatalf("disconnect probed prior authentication: %q, %v", probes, err)
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
	conn := open()
	if event := next(conn); event.Type != "ready" {
		t.Fatalf("expected a ready terminal, got %+v", event)
	}
	if event := next(conn); event.Type != "output" || !strings.Contains(event.Data, "Use a Google Cloud project") {
		t.Fatalf("expected the native Cloud prompt, got %+v", event)
	}
	mark("release-refresh")
	select {
	case result := <-disconnected:
		defaults := result.Settings.ACP[acp.AgentAntigravity]
		status, ok := result.Settings.ACPAuth[acp.AgentAntigravity]
		if result.Err != nil || defaults.Enabled || defaults.Auth.Mode != acp.AuthModeAuto || !ok || status.Authenticated {
			t.Fatalf("expected disabled and signed-out settings, got %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect did not return after the status probe")
	}
	if probes, err := os.ReadFile(filepath.Join(home, "probes")); err != nil || len(probes) != 2 {
		t.Fatalf("expected one settings probe and one sign-in probe, got %q, %v", probes, err)
	}
	if event := next(open()); event.Type != "error" || event.Err != errAntigravityAuthBusy.Error() {
		t.Fatalf("expected a concurrent login rejection, got %+v", event)
	}
	if err := conn.WriteJSON(terminalClientMessage{Type: "input", Data: "test-project\r"}); err != nil {
		t.Fatal(err)
	}
	defer mark("release-verification")
	waitFor("verifying")
	if err := conn.WriteJSON(terminalClientMessage{Type: "resize", Cols: 81, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	blocked, err := server.Client().Do(disconnect)
	if err != nil {
		t.Fatal(err)
	}
	_ = blocked.Body.Close()
	if blocked.StatusCode != http.StatusConflict {
		t.Fatalf("disconnect during sign-in status = %d", blocked.StatusCode)
	}
	mark("release-verification")
	for {
		event := next(conn)
		if event.Type != "output" {
			if event.Type != "exit" || event.Code != 0 || event.Err != "" {
				t.Fatalf("expected verified exit, got %+v", event)
			}
			break
		}
	}
}
