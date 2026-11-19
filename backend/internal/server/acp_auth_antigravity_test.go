package server

import (
	"encoding/json"
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
    printf '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":%s,"authMethods":[{"id":"oauth-business","name":"Cloud"},{"id":"oauth-personal","name":"Google"}]}}\n' "$version"
    ;;
models)
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
	server := httptest.NewServer((&Server{Root: t.TempDir(), AuthKey: "test-key"}).Handler())
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
	conn := open()
	if event := next(conn); event.Type != "ready" {
		t.Fatalf("expected a ready terminal, got %+v", event)
	}
	if event := next(conn); event.Type != "output" || !strings.Contains(event.Data, "Use a Google Cloud project") {
		t.Fatalf("expected the native Cloud prompt, got %+v", event)
	}
	if event := next(open()); event.Type != "error" || event.Err != errAntigravityAuthBusy.Error() {
		t.Fatalf("expected a concurrent login rejection, got %+v", event)
	}
	if err := conn.WriteJSON(terminalClientMessage{Type: "input", Data: "test-project\r"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(home, "verifying")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("terminal exited without verifying authentication")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := conn.WriteJSON(terminalClientMessage{Type: "resize", Cols: 81, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	disconnect, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+path+"/disconnect", nil)
	if err != nil {
		t.Fatal(err)
	}
	disconnect.Header = request.Header.Clone()
	blocked, err := server.Client().Do(disconnect)
	if err != nil {
		t.Fatal(err)
	}
	_ = blocked.Body.Close()
	if blocked.StatusCode != http.StatusConflict {
		t.Fatalf("disconnect during sign-in status = %d", blocked.StatusCode)
	}
	if err := os.WriteFile(filepath.Join(home, "release-verification"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
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
