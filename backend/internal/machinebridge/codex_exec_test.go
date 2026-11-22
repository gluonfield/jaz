package machinebridge

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestHelperCodexExecServer acts as a fake `codex exec-server` subprocess when
// JAZ_TEST_FAKE_CODEX_EXEC=1 is set in the environment.
func TestHelperCodexExecServer(t *testing.T) {
	if os.Getenv("JAZ_TEST_FAKE_CODEX_EXEC") != "1" {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer ln.Close()

	if wd, err := os.Getwd(); err == nil {
		_ = os.WriteFile(filepath.Join(wd, ".exec-server-started"), []byte(ln.Addr().String()), 0o600)
	}
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
	t.Setenv("JAZ_TEST_FAKE_CODEX_EXEC", "1")
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

func TestCodexExecLauncherStartStopAndConfinement(t *testing.T) {
	base := t.TempDir()
	grantDir := filepath.Join(base, "work")
	outsideDir := filepath.Join(base, "outside")
	if err := os.MkdirAll(filepath.Join(grantDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(grantDir, "escape-link")); err != nil {
		t.Fatal(err)
	}

	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "work", Mode: AccessReadWrite, RootPath: grantDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	launcher := newTestCodexLauncher(t, guard)
	ctx := t.Context()

	// Unknown grant rejected.
	if _, err := launcher.Start(ctx, ExecStartRequest{GrantID: "missing", RunID: "r1"}, ""); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("unknown grant err = %v, want ErrAccessDenied", err)
	}
	// Traversal in ReadOnlyDirs rejected.
	if _, err := launcher.Start(ctx, ExecStartRequest{GrantID: "work", RunID: "r2", ReadOnlyDirs: []string{"../outside"}}, ""); !errors.Is(err, ErrPathOutsideGrant) {
		t.Fatalf("traversal ro dir err = %v, want ErrPathOutsideGrant", err)
	}
	// Symlink escape in ReadOnlyDirs rejected.
	if _, err := launcher.Start(ctx, ExecStartRequest{GrantID: "work", RunID: "r3", ReadOnlyDirs: []string{"escape-link"}}, ""); !errors.Is(err, ErrSymlinkEscape) {
		t.Fatalf("symlink escape ro dir err = %v, want ErrSymlinkEscape", err)
	}
	if launcher.ActiveCount() != 0 {
		t.Fatalf("rejected starts left %d active sessions", launcher.ActiveCount())
	}

	// Valid start inside grant root spawns process, verifies loopback endpoint, and sets cwd to grant root.
	started, err := launcher.Start(ctx, ExecStartRequest{GrantID: "work", RunID: "r-ok", ReadOnlyDirs: []string{"sub"}}, "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !strings.HasPrefix(started.SessionID, "exec-") || !strings.HasPrefix(started.Endpoint, "ws://127.0.0.1:") {
		t.Fatalf("unexpected ExecStartResponse: %+v", started)
	}
	if launcher.ActiveCount() != 1 {
		t.Fatalf("ActiveCount = %d, want 1", launcher.ActiveCount())
	}
	marker, err := os.ReadFile(filepath.Join(grantDir, ".exec-server-started"))
	if err != nil || !strings.HasSuffix(started.Endpoint, string(marker)) {
		t.Fatalf("helper cwd marker = (%q, %v), endpoint = %q", string(marker), err, started.Endpoint)
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

func TestCodexExecLauncherCloseKillsAllSessions(t *testing.T) {
	dir := t.TempDir()
	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "work", Mode: AccessReadWrite, RootPath: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	launcher := newTestCodexLauncher(t, guard)
	ctx := t.Context()

	s1, err := launcher.Start(ctx, ExecStartRequest{GrantID: "work", RunID: "r1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := launcher.Start(ctx, ExecStartRequest{GrantID: "work", RunID: "r2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if launcher.ActiveCount() != 2 {
		t.Fatalf("ActiveCount = %d, want 2", launcher.ActiveCount())
	}

	// Verify Client.Close() closes the launcher and terminates all active sessions.
	client, err := NewClient(ClientConfig{
		CloudURL:     "https://cloud.example.com",
		Token:        "tok",
		DeviceID:     "dev-close",
		Capabilities: Capabilities{LocalExec: true},
		FolderGrants: []FolderGrant{{ID: "work", Mode: AccessReadWrite, RootPath: dir}},
		ExecLauncher: launcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Client.Close failed: %v", err)
	}
	if launcher.ActiveCount() != 0 {
		t.Fatalf("ActiveCount after Close = %d, want 0", launcher.ActiveCount())
	}
	for _, ep := range []string{s1.Endpoint, s2.Endpoint} {
		parsed, _ := url.Parse(ep)
		if conn, err := net.DialTimeout("tcp", parsed.Host, 200*time.Millisecond); err == nil {
			_ = conn.Close()
			t.Fatalf("session %s still listening after Close", ep)
		}
	}
	if _, err := launcher.Start(ctx, ExecStartRequest{GrantID: "work", RunID: "r3"}, ""); err == nil {
		t.Fatal("expected Start after Close to fail")
	}
}

func TestCodexExecLauncherRealBinary(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex binary not installed")
	}
	dir := t.TempDir()
	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "work", Mode: AccessReadWrite, RootPath: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := NewCodexExecLauncher(CodexExecConfig{
		Guard:          guard,
		StartupTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = launcher.Close() })

	started, err := launcher.Start(t.Context(), ExecStartRequest{GrantID: "work", RunID: "real-codex-1"}, "")
	if err != nil {
		t.Fatalf("real codex exec-server Start failed: %v", err)
	}
	if !strings.HasPrefix(started.Endpoint, "ws://127.0.0.1:") {
		t.Fatalf("unexpected real codex endpoint: %+v", started)
	}
	if err := launcher.Stop(t.Context(), ExecStopRequest{SessionID: started.SessionID}); err != nil {
		t.Fatalf("real codex exec-server Stop failed: %v", err)
	}
}
