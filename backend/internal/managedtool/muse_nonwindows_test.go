//go:build !windows

package managedtool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestMuseCancellationStopsInstallerChildrenAndAllowsRetry(t *testing.T) {
	env := museTestEnv(t)
	var requests atomic.Int32
	installer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `sleep 60 &
echo $! > "$MUSE_INSTALL_DIR/../child.pid"
wait
`
		if requests.Add(1) > 1 {
			body = `printf 'native launcher' > "$MUSE_INSTALL_DIR/muse"
chmod 755 "$MUSE_INSTALL_DIR/muse"
`
		}
		_, _ = w.Write([]byte(body))
	}))
	defer installer.Close()
	root := t.TempDir()
	manager := New(root)
	manager.museInstallerURL = installer.URL + "/install.sh"
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- manager.Prepare(ctx, MuseCLI, env)
	}()
	pidFile := filepath.Join(filepath.Dir(filepath.Dir(ExecutablePath(root, MuseCLI))), "child.pid")
	var pid int
	for deadline := time.Now().Add(5 * time.Second); pid == 0; {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		if time.Now().After(deadline) {
			t.Fatal("installer did not start its child")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled installation succeeded")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("cancelled installer kept the installation lock or output pipes open")
	}
	for deadline := time.Now().Add(2 * time.Second); ; {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("installer child %d survived cancellation", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status := manager.Status(MuseCLI, env); status.State != StateFailed {
		t.Fatalf("cancelled installation status = %#v", status)
	}
	retry, stop := context.WithTimeout(t.Context(), 2*time.Second)
	defer stop()
	if err := manager.Prepare(retry, MuseCLI, env); err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
	if status := manager.Status(MuseCLI, env); status.State != StateReady {
		t.Fatalf("retry status = %#v", status)
	}
}
