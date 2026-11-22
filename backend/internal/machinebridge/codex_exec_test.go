package machinebridge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestHelperCodexExecServer acts as a fake `codex exec-server` subprocess when
// TEST_FAKE_CODEX_EXEC=1 is set in the environment.
func TestHelperCodexExecServer(t *testing.T) {
	if os.Getenv("TEST_FAKE_CODEX_EXEC") != "1" {
		return
	}
	if delay := os.Getenv("TEST_FAKE_CODEX_DELAY_MS"); delay != "" {
		time.Sleep(250 * time.Millisecond)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer ln.Close()

	if wd, err := os.Getwd(); err == nil {
		_ = os.WriteFile(filepath.Join(wd, ".exec-server-started"), []byte(ln.Addr().String()), 0o600)
		_ = os.WriteFile(filepath.Join(wd, ".exec-server-jaz-token"), []byte(os.Getenv("JAZ_CLOUD_TOKEN")), 0o600)
	}
	// Emit a >64 KiB stdout line first to verify scanner/reader handles huge lines cleanly.
	fmt.Println(strings.Repeat("x", 80*1024))
	fmt.Printf("ws://%s\n", ln.Addr().String())

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	os.Exit(0)
}

func newTestCodexLauncher(t *testing.T, guard *PolicyGuard) *CodexExecLauncher {
	t.Helper()
	t.Setenv("TEST_FAKE_CODEX_EXEC", "1")
	launcher, err := NewCodexExecLauncher(CodexExecConfig{
		Guard:          guard,
		Binary:         os.Args[0],
		Args:           []string{"-test.run=^TestHelperCodexExecServer$"},
		StartupTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = launcher.Close() })
	return launcher
}

func TestCodexExecLauncherStartStopPermissionsAndEnvFiltering(t *testing.T) {
	base := t.TempDir()
	roDir := filepath.Join(base, "ro")
	grantDir := filepath.Join(base, "work")
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(grantDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("JAZ_CLOUD_TOKEN", "secret-cloud-bearer-token")
	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "ro", Mode: AccessReadOnly, RootPath: roDir},
		{ID: "work", Mode: AccessReadWrite, RootPath: grantDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.Close() })
	launcher := newTestCodexLauncher(t, guard)
	ctx := t.Context()

	// Unknown grant rejected.
	if _, err := launcher.Start(ctx, ExecStartRequest{GrantID: "missing", RunID: "r1"}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("unknown grant err = %v, want ErrAccessDenied", err)
	}
	// Read-only grant rejected (requires OpWrite).
	if _, err := launcher.Start(ctx, ExecStartRequest{GrantID: "ro", RunID: "r-ro"}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("ro grant err = %v, want ErrAccessDenied", err)
	}
	// ReadOnlyDirs fails closed since codex exec-server cannot enforce it natively.
	if _, err := launcher.Start(ctx, ExecStartRequest{GrantID: "work", RunID: "r-rodirs", ReadOnlyDirs: []string{"sub"}}); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("ReadOnlyDirs err = %v, want ErrUnsupportedCapability", err)
	}
	if launcher.ActiveCount() != 0 {
		t.Fatalf("rejected starts left %d active sessions", launcher.ActiveCount())
	}

	// Valid start inside rw grant root spawns process, strips JAZ_* env, handles >64KiB stdout, and returns AuthToken.
	started, err := launcher.Start(ctx, ExecStartRequest{GrantID: "work", RunID: "r-ok"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !strings.HasPrefix(started.SessionID, "exec-") || !strings.HasPrefix(started.Endpoint, "ws://127.0.0.1:") || len(started.AuthToken) != 64 {
		t.Fatalf("unexpected ExecStartResponse: %+v", started)
	}
	if launcher.ActiveCount() != 1 {
		t.Fatalf("ActiveCount = %d, want 1", launcher.ActiveCount())
	}
	marker, err := os.ReadFile(filepath.Join(grantDir, ".exec-server-started"))
	if err != nil || !strings.HasSuffix(started.Endpoint, string(marker)) {
		t.Fatalf("helper cwd marker = (%q, %v), endpoint = %q", string(marker), err, started.Endpoint)
	}
	leakedToken, err := os.ReadFile(filepath.Join(grantDir, ".exec-server-jaz-token"))
	if err != nil || len(leakedToken) != 0 {
		t.Fatalf("JAZ_CLOUD_TOKEN leaked to child env: %q (err=%v)", string(leakedToken), err)
	}

	parsed, err := url.Parse(started.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", parsed.Host, time.Second)
	if err != nil {
		t.Fatalf("endpoint %s not listening: %v", started.Endpoint, err)
	}
	_ = conn.Close()

	// Stop terminates subprocess and closes listener.
	if err := launcher.Stop(ctx, ExecStopRequest{SessionID: started.SessionID}); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if launcher.ActiveCount() != 0 {
		t.Fatalf("ActiveCount after Stop = %d, want 0", launcher.ActiveCount())
	}
	if conn, err := net.DialTimeout("tcp", parsed.Host, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatalf("stopped exec-server still listening on %s", parsed.Host)
	}
	if err := launcher.Stop(ctx, ExecStopRequest{SessionID: started.SessionID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Stop err = %v, want ErrNotFound", err)
	}
}

func TestCodexExecLauncherCloseWaitsForInFlightStartAndKillsAllSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TEST_FAKE_CODEX_EXEC", "1")
	t.Setenv("TEST_FAKE_CODEX_DELAY_MS", "250")

	var capturedLauncher *CodexExecLauncher
	client, err := NewClient(ClientConfig{
		CloudURL:     "https://cloud.example.com",
		Token:        "tok",
		DeviceID:     "dev-close",
		Capabilities: Capabilities{LocalExec: true},
		FolderGrants: []FolderGrant{{ID: "work", Mode: AccessReadWrite, RootPath: dir}},
		ExecLauncherFactory: func(guard *PolicyGuard) (ExecLauncher, error) {
			l, err := NewCodexExecLauncher(CodexExecConfig{
				Guard:          guard,
				Binary:         os.Args[0],
				Args:           []string{"-test.run=^TestHelperCodexExecServer$"},
				StartupTimeout: 5 * time.Second,
			})
			capturedLauncher = l
			return l, err
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	s1, err := capturedLauncher.Start(t.Context(), ExecStartRequest{GrantID: "work", RunID: "r1"})
	if err != nil {
		t.Fatal(err)
	}

	// Launch an in-flight Start concurrently with Client.Close() to ensure Close waits and leaves zero orphans.
	inFlightDone := make(chan error, 1)
	go func() {
		_, err := capturedLauncher.Start(context.Background(), ExecStartRequest{GrantID: "work", RunID: "r-inflight"})
		inFlightDone <- err
	}()
	time.Sleep(50 * time.Millisecond)

	if err := client.Close(); err != nil {
		t.Fatalf("Client.Close failed: %v", err)
	}
	select {
	case err := <-inFlightDone:
		if err == nil {
			t.Fatal("in-flight Start unexpectedly succeeded after Close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for in-flight Start to exit after Close")
	}
	if capturedLauncher.ActiveCount() != 0 {
		t.Fatalf("ActiveCount after Close = %d, want 0", capturedLauncher.ActiveCount())
	}
	parsed, _ := url.Parse(s1.Endpoint)
	if conn, err := net.DialTimeout("tcp", parsed.Host, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatalf("session %s still listening after Close", s1.Endpoint)
	}
	if _, err := capturedLauncher.Start(t.Context(), ExecStartRequest{GrantID: "work", RunID: "r3"}); err == nil {
		t.Fatal("expected Start after Close to fail")
	}
}

func TestCodexExecLauncherRealBinary(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex binary not installed")
	}
	base := t.TempDir()
	roDir := filepath.Join(base, "ro")
	rwDir := filepath.Join(base, "rw")
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rwDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAZ_CLOUD_TOKEN", "do-not-leak-to-codex")

	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "ro", Mode: AccessReadOnly, RootPath: roDir},
		{ID: "work", Mode: AccessReadWrite, RootPath: rwDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.Close() })

	launcher, err := NewCodexExecLauncher(CodexExecConfig{
		Guard:          guard,
		StartupTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = launcher.Close() })

	if _, err := launcher.Start(t.Context(), ExecStartRequest{GrantID: "ro", RunID: "real-ro"}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("real codex ro grant err = %v, want ErrAccessDenied", err)
	}
	if _, err := launcher.Start(t.Context(), ExecStartRequest{GrantID: "work", RunID: "real-rodirs", ReadOnlyDirs: []string{"sub"}}); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("real codex ReadOnlyDirs err = %v, want ErrUnsupportedCapability", err)
	}

	started, err := launcher.Start(t.Context(), ExecStartRequest{GrantID: "work", RunID: "real-codex-1"})
	if err != nil {
		t.Fatalf("real codex exec-server Start failed: %v", err)
	}
	if !strings.HasPrefix(started.Endpoint, "ws://127.0.0.1:") || len(started.AuthToken) != 64 {
		t.Fatalf("unexpected real codex response: %+v", started)
	}

	// Unauthenticated WS dial must be rejected (401 Unauthorized).
	if ws, resp, err := websocket.DefaultDialer.Dial(started.Endpoint, nil); err == nil {
		_ = ws.Close()
		t.Fatal("real codex exec-server accepted unauthenticated WebSocket connection")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("unauthenticated WS status = %d, err = %v; want 401", status, err)
	}

	// Wrong bearer token must be rejected (401 Unauthorized).
	wrongHdr := http.Header{"Authorization": {"Bearer " + strings.Repeat("0", 64)}}
	if ws, resp, err := websocket.DefaultDialer.Dial(started.Endpoint, wrongHdr); err == nil {
		_ = ws.Close()
		t.Fatal("real codex exec-server accepted wrong capability token")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("wrong token WS status = %d, err = %v; want 401", status, err)
	}

	// Valid capability token must succeed.
	validHdr := http.Header{"Authorization": {"Bearer " + started.AuthToken}}
	ws, _, err := websocket.DefaultDialer.Dial(started.Endpoint, validHdr)
	if err != nil {
		t.Fatalf("authenticated WS dial to real codex exec-server failed: %v", err)
	}
	_ = ws.Close()

	// StopAll terminates active sessions cleanly (simulating disconnect).
	launcher.StopAll()
	if launcher.ActiveCount() != 0 {
		t.Fatalf("ActiveCount after StopAll = %d, want 0", launcher.ActiveCount())
	}
	parsed, _ := url.Parse(started.Endpoint)
	if conn, err := net.DialTimeout("tcp", parsed.Host, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatalf("real codex exec-server still listening after StopAll")
	}
}
