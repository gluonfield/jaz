package managedtool

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPendingInstallDoesNotBlockReadyToolsOrCancelledRequests(t *testing.T) {
	env := museTestEnv(t)
	started := make(chan struct{})
	installer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer installer.Close()
	manager := New(t.TempDir())
	manager.museInstallerURL = installer.URL + "/install.sh"
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	active := make(chan error, 1)
	go func() {
		active <- manager.Prepare(ctx, MuseCLI, env)
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("installer did not start")
	}
	cancelled, stop := context.WithCancel(t.Context())
	defer stop()
	done := make(chan error, 1)
	go func() {
		done <- manager.Prepare(cancelled, MuseCLI, env)
	}()
	select {
	case err := <-done:
		t.Fatalf("pending installation returned before cancellation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled wait = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request waited for the active download")
	}
	selected := filepath.Join(t.TempDir(), ExecutableName(MuseCLI))
	if err := os.WriteFile(selected, []byte("existing native launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	ready := make(chan error, 1)
	go func() {
		ready <- manager.Prepare(ctx, MuseCLI, map[string]string{"MUSE_CLI": selected})
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("ready native tool required installation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ready native tool waited for an unrelated installation")
	}
	cancel()
	if err := <-active; !errors.Is(err, context.Canceled) {
		t.Fatalf("active download cancellation = %v", err)
	}
}
