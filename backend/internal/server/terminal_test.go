package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"github.com/wins/jaz/backend/internal/terminal"
)

func TestSessionTerminalRequiresCWD(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "no-cwd"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&Server{Store: store, Terminal: terminal.New()}).Handler())
	defer server.Close()

	conn, res, err := websocket.DefaultDialer.Dial(wsURL(server.URL, "/v1/sessions/"+session.ID+"/terminal"), nil)
	if err == nil {
		conn.Close()
		t.Fatal("terminal websocket opened for session without cwd")
	}
	if res == nil || res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %v, want %d, err = %v", responseStatus(res), http.StatusBadRequest, err)
	}
}

func TestSessionTerminalDoesNotStartBeforeWebsocketUpgrade(t *testing.T) {
	cwd := t.TempDir()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       "terminal-upgrade",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: cwd},
	})
	if err != nil {
		t.Fatal(err)
	}
	terminals := terminal.New()
	srv := &Server{Store: store, Terminal: terminals}
	defer terminals.Close()
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	res, err := http.Get(server.URL + "/v1/sessions/" + session.ID + "/terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if terminals.Active(session.ID) {
		t.Fatal("terminal started before websocket upgrade")
	}
}

func TestSessionTerminalAcceptsQueryKeyWithBackendAuth(t *testing.T) {
	cwd := t.TempDir()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       "terminal-auth",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: cwd},
	})
	if err != nil {
		t.Fatal(err)
	}
	terminals := terminal.New()
	srv := &Server{Store: store, Terminal: terminals, AuthKey: "secret"}
	defer terminals.Close()
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	path := "/v1/sessions/" + session.ID + "/terminal?cols=80&rows=24"
	conn, res, err := websocket.DefaultDialer.Dial(wsURL(server.URL, path), nil)
	if err == nil {
		conn.Close()
		t.Fatal("terminal websocket opened without backend key")
	}
	if res == nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want %d, err = %v", responseStatus(res), http.StatusUnauthorized, err)
	}
	if terminals.Active(session.ID) {
		t.Fatal("terminal started before auth")
	}

	conn, _, err = websocket.DefaultDialer.Dial(wsURL(server.URL, path+"&key=secret"), http.Header{"Accept-Encoding": []string{"gzip"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var ready terminal.Message
	readMessage(t, conn, &ready)
	if ready.Type != "ready" || ready.Cwd != cwd {
		t.Fatalf("ready = %#v, want cwd %q", ready, cwd)
	}
}

func TestSessionTerminalStartsInCWDAndReplaysOnReconnect(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(cwd+"/marker.txt", []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shellCwd := cwd
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		shellCwd = resolved
	}
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       "terminal-cwd",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: cwd},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, Terminal: terminal.New()}
	defer srv.Terminal.Close()
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	conn := dialTerminal(t, server.URL, session.ID)
	defer conn.Close()
	var ready terminal.Message
	readMessage(t, conn, &ready)
	if ready.Type != "ready" || ready.Cwd != cwd {
		t.Fatalf("ready = %#v, want cwd %q", ready, cwd)
	}
	if err := conn.WriteJSON(map[string]any{"type": "input", "data": "printf 'JAZPWD:'; pwd\n"}); err != nil {
		t.Fatal(err)
	}
	readUntilOutput(t, conn, "JAZPWD:"+shellCwd)
	conn.Close()

	conn = dialTerminal(t, server.URL, session.ID)
	defer conn.Close()
	readMessage(t, conn, &ready)
	replay := readUntilOutput(t, conn, "JAZPWD:"+shellCwd)
	if !strings.Contains(replay, "JAZPWD:"+shellCwd) {
		t.Fatalf("replay = %q", replay)
	}
}

func dialTerminal(t *testing.T, baseURL, sessionID string) *websocket.Conn {
	t.Helper()
	header := http.Header{"Accept-Encoding": []string{"gzip"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(baseURL, "/v1/sessions/"+sessionID+"/terminal?cols=80&rows=24"), header)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func readMessage(t *testing.T, conn *websocket.Conn, out *terminal.Message) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := conn.ReadJSON(out); err != nil {
		t.Fatal(err)
	}
}

func readUntilOutput(t *testing.T, conn *websocket.Conn, want string) string {
	t.Helper()
	var out strings.Builder
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var msg terminal.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("terminal output %q did not contain %q: %v", out.String(), want, err)
		}
		if msg.Type == "output" {
			out.WriteString(msg.Data)
			if strings.Contains(out.String(), want) {
				return out.String()
			}
		}
	}
}

func wsURL(baseURL, path string) string {
	return "ws" + strings.TrimPrefix(baseURL, "http") + path
}

func responseStatus(res *http.Response) int {
	if res == nil {
		return 0
	}
	return res.StatusCode
}
