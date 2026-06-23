package acpadapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveAdapterDownloadsVerifiesAndExtractsManifestArchive(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testTarball(t, map[string]string{"bin/tool": "ok", "bin/claude": "sdk"})
	sum := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_, _ = fmt.Fprintf(w, `{"adapters":{"claude":{"version":"1.2.3","assets":{"%s":{"url":"%s","sha256":"%x","binary":"bin/tool","env":{"CLAUDE_CODE_EXECUTABLE":"bin/claude"}}}}}}`, platform, serverURL(r, "/claude.tgz"), sum)
		case "/claude.tgz":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL+"/manifest.json", server.Client())
	launch, err := manager.ResolveAdapter(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(launch.Command) != "tool" {
		t.Fatalf("command = %q", launch.Command)
	}
	if got := launch.Env["CLAUDE_CODE_EXECUTABLE"]; filepath.Base(got) != "claude" {
		t.Fatalf("claude executable env = %q", got)
	}
	got, err := os.ReadFile(launch.Command)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Fatalf("binary = %q", got)
	}
	status := manager.Status("claude")
	if status.State != StateReady || status.Version != "1.2.3" || status.Platform != platform {
		t.Fatalf("status = %#v", status)
	}
}

func TestResolveAdapterRejectsBadChecksum(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testTarball(t, map[string]string{"tool": "ok"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_, _ = fmt.Fprintf(w, `{"adapters":{"codex":{"version":"1.2.3","assets":{"%s":{"url":"%s","sha256":"%064x","binary":"tool"}}}}}`, platform, serverURL(r, "/codex.tgz"), 1)
		case "/codex.tgz":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL+"/manifest.json", server.Client())
	if _, err := manager.ResolveAdapter(context.Background(), "codex"); err == nil {
		t.Fatal("expected checksum error")
	}
	status := manager.Status("codex")
	if status.State != StateFailed || status.Message == "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestResolveAdapterRejectsUnsafeArchivePath(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testTarball(t, map[string]string{"../tool": "bad", "tool": "ok"})
	sum := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_, _ = fmt.Fprintf(w, `{"adapters":{"codex":{"version":"1.2.3","assets":{"%s":{"url":"%s","sha256":"%x","binary":"tool"}}}}}`, platform, serverURL(r, "/codex.tgz"), sum)
		case "/codex.tgz":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL+"/manifest.json", server.Client())
	if _, err := manager.ResolveAdapter(context.Background(), "codex"); err == nil {
		t.Fatal("expected unsafe path error")
	}
}

func TestStatusReportsMissingBeforeManifestDownload(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	status := New(t.TempDir()).Status("codex")
	if status.State != StateMissing || status.Platform != platform {
		t.Fatalf("status = %#v", status)
	}
}

func TestPrepareRecordsFailedDownload(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL+"/manifest.json", server.Client())
	if err := manager.Prepare(context.Background(), "codex"); err == nil {
		t.Fatal("expected prepare error")
	}
	status := manager.Status("codex")
	if status.State != StateFailed || status.Message == "" {
		t.Fatalf("failed status = %#v", status)
	}
}

func TestResolveAdapterReusesInstalledBinary(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "acp", "managed", "adapters", "codex", "1.2.3", platform, "tool")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manifest.json" {
			t.Fatalf("unexpected archive download %s", r.URL.Path)
		}
		_, _ = fmt.Fprintf(w, `{"adapters":{"codex":{"version":"1.2.3","assets":{"%s":{"url":"%s","sha256":"%064x","binary":"tool"}}}}}`, platform, serverURL(r, "/codex.tgz"), 1)
	}))
	defer server.Close()

	manager := NewForTest(root, server.URL+"/manifest.json", server.Client())
	launch, err := manager.ResolveAdapter(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	if launch.Command != path {
		t.Fatalf("command = %q, want %q", launch.Command, path)
	}
}

func TestResolveAdapterUsesDiskManifestCacheWhenRemoteFails(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testTarball(t, map[string]string{"tool": "ok"})
	sum := sha256.Sum256(body)
	remoteFails := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if remoteFails {
			http.Error(w, "offline", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/manifest.json":
			_, _ = fmt.Fprintf(w, `{"adapters":{"codex":{"version":"1.2.3","assets":{"%s":{"url":"%s","sha256":"%x","binary":"tool"}}}}}`, platform, serverURL(r, "/codex.tgz"), sum)
		case "/codex.tgz":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	first := NewForTest(root, server.URL+"/manifest.json", server.Client())
	launch, err := first.ResolveAdapter(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	remoteFails = true
	second := NewForTest(root, server.URL+"/manifest.json", server.Client())
	cachedLaunch, err := second.ResolveAdapter(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	if cachedLaunch.Command != launch.Command {
		t.Fatalf("cached command = %q, want %q", cachedLaunch.Command, launch.Command)
	}
}

func testTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serverURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}
