package managedtool

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestPrepareMuseInstallerFailureCanBeRetried(t *testing.T) {
	root := t.TempDir()
	var fail atomic.Bool
	fail.Store(true)
	installer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "download unavailable", http.StatusServiceUnavailable)
			return
		}
		body := `set -eu
test "$MUSE_NO_MODIFY_PATH" = 1
test "$MUSE_LOGIN" = 0
mkdir -p "$MUSE_INSTALL_DIR"
printf 'native launcher' > "$MUSE_INSTALL_DIR/muse"
chmod 755 "$MUSE_INSTALL_DIR/muse"
`
		if runtime.GOOS == "windows" {
			body = `$ErrorActionPreference = 'Stop'
if ($env:MUSE_NO_MODIFY_PATH -ne '1' -or $env:MUSE_LOGIN -ne '0') {
  exit 1
}
New-Item -ItemType Directory -Force -Path $env:MUSE_INSTALL_DIR | Out-Null
Set-Content -Path (Join-Path $env:MUSE_INSTALL_DIR 'muse.cmd') -Value 'native launcher'
`
		}
		_, _ = w.Write([]byte(body))
	}))
	defer installer.Close()
	manager := New(root)
	manager.museInstallerURL = installer.URL + "/install.sh"
	if err := manager.Prepare(t.Context(), MuseCLI, nil); err == nil {
		t.Fatal("installer download failure was ignored")
	}
	if status := manager.Status(MuseCLI, nil); status.State != StateFailed {
		t.Fatalf("failed installation status = %#v", status)
	}
	fail.Store(false)
	if err := manager.Prepare(t.Context(), MuseCLI, nil); err != nil {
		t.Fatal(err)
	}
	status := manager.Status(MuseCLI, nil)
	if status.State != StateReady || status.Path != ExecutablePath(root, MuseCLI) {
		t.Fatalf("installed status = %#v", status)
	}
	data, err := os.ReadFile(status.Path)
	if err != nil || len(data) == 0 {
		t.Fatalf("native launcher missing: %q, %v", data, err)
	}
	installer.Close()
	if err := manager.Prepare(t.Context(), MuseCLI, nil); err != nil {
		t.Fatalf("installed launcher required another download: %v", err)
	}
}

func TestPrepareMuseRejectsInstallerWithoutLauncher(t *testing.T) {
	installer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("exit 0\n"))
	}))
	defer installer.Close()
	manager := New(t.TempDir())
	manager.museInstallerURL = installer.URL + "/install.sh"
	if err := manager.Prepare(t.Context(), MuseCLI, nil); err == nil {
		t.Fatal("an installer that produced no launcher was accepted")
	}
	if status := manager.Status(MuseCLI, nil); status.State != StateFailed {
		t.Fatalf("missing-launcher status = %#v", status)
	}
}

func TestPrepareMuseReusesExistingCLI(t *testing.T) {
	bin := t.TempDir()
	path := filepath.Join(bin, ExecutableName(MuseCLI))
	if err := os.WriteFile(path, []byte("native launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	manager := New(t.TempDir())
	manager.museInstallerURL = "http://127.0.0.1:1"
	if err := manager.Prepare(t.Context(), MuseCLI, nil); err != nil {
		t.Fatal(err)
	}
	if status := manager.Status(MuseCLI, nil); status.State != StateReady || status.Path != path {
		t.Fatalf("existing CLI status = %#v", status)
	}
}

func TestMuseFailureDoesNotPublishPartialLauncher(t *testing.T) {
	installer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `mkdir -p "$MUSE_INSTALL_DIR"
printf 'partial launcher' > "$MUSE_INSTALL_DIR/muse"
exit 1
`
		if runtime.GOOS == "windows" {
			body = `New-Item -ItemType Directory -Force -Path $env:MUSE_INSTALL_DIR | Out-Null
Set-Content -Path (Join-Path $env:MUSE_INSTALL_DIR 'muse.cmd') -Value 'partial launcher'
exit 1
`
		}
		_, _ = w.Write([]byte(body))
	}))
	defer installer.Close()
	root := t.TempDir()
	manager := New(root)
	manager.museInstallerURL = installer.URL + "/install.sh"
	if err := manager.Prepare(t.Context(), MuseCLI, nil); err == nil {
		t.Fatal("partial installation succeeded")
	}
	if status := manager.Status(MuseCLI, nil); status.State != StateFailed {
		t.Fatalf("partial launcher became ready: %#v", status)
	}
	if status := New(root).Status(MuseCLI, nil); status.State != StateMissing {
		t.Fatalf("partial launcher survived restart as installed: %#v", status)
	}
}
