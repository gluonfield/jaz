package acpadapter

import (
	"archive/tar"
	"archive/zip"
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
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResolveAdapterDownloadsVerifiesAndExtractsManifestArchive(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testTarballMode(t, map[string]string{"bin/tool": "ok", "bin/claude": "sdk"}, 0o644)
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

	root := t.TempDir()
	incomplete := filepath.Join(root, "acp", "managed", "adapters", "claude", "1.2.3", platform, "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(incomplete), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(incomplete, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := NewForTest(root, server.URL+"/manifest.json", server.Client())
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
	for _, executable := range []string{launch.Command, launch.Env["CLAUDE_CODE_EXECUTABLE"]} {
		info, err := os.Stat(executable)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("mode for %s = %v, want executable", executable, info.Mode().Perm())
		}
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

func TestResolveAdapterExtractsZipArchive(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	binary := "kimi"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	body := testZip(t, map[string]string{binary: "ok"})
	sum := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_, _ = fmt.Fprintf(w, `{"adapters":{"kimi":{"version":"0.28.0","assets":{"%s":{"url":"%s","sha256":"%x","binary":"%s"}}}}}`, platform, serverURL(r, "/kimi.zip"), sum, binary)
		case "/kimi.zip":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	launch, err := NewForTest(t.TempDir(), server.URL+"/manifest.json", server.Client()).ResolveAdapter(context.Background(), "kimi")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(launch.Command); err != nil || string(got) != "ok" {
		t.Fatalf("installed Kimi binary = %q, %v", got, err)
	}
	if info, err := os.Stat(launch.Command); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed Kimi mode = %v, %v", info, err)
	}
}

func TestResolveAdapterReportsDownloadProgress(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testTarball(t, map[string]string{"bin/tool": strings.Repeat("ok", 1024)})
	sum := sha256.Sum256(body)
	firstChunk := len(body) / 2
	wroteFirst := make(chan struct{})
	continueDownload := make(chan struct{})
	var releaseOnce sync.Once
	releaseDownload := func() {
		releaseOnce.Do(func() {
			close(continueDownload)
		})
	}
	defer releaseDownload()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_, _ = fmt.Fprintf(w, `{"adapters":{"claude":{"version":"1.2.3","assets":{"%s":{"url":"%s","sha256":"%x","binary":"bin/tool"}}}}}`, platform, serverURL(r, "/claude.tgz"), sum)
		case "/claude.tgz":
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body[:firstChunk])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(wroteFirst)
			<-continueDownload
			_, _ = w.Write(body[firstChunk:])
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL+"/manifest.json", server.Client())
	done := make(chan error, 1)
	go func() {
		_, err := manager.ResolveAdapter(context.Background(), "claude")
		done <- err
	}()

	select {
	case <-wroteFirst:
	case <-time.After(2 * time.Second):
		t.Fatal("adapter archive download did not start")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := manager.Status("claude")
		if status.State == StateDownloading && status.BytesDownloaded > 0 {
			if status.BytesDownloaded >= int64(len(body)) || status.BytesTotal != int64(len(body)) {
				t.Fatalf("progress bytes = downloaded %d total %d, want partial total %d", status.BytesDownloaded, status.BytesTotal, len(body))
			}
			if status.ProgressPercent <= 0 || status.ProgressPercent >= 100 {
				t.Fatalf("progress percent = %d, want partial", status.ProgressPercent)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status never reported progress: %#v", manager.Status("claude"))
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseDownload()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("adapter download did not finish")
	}
}

func TestResolveAdapterPrefersLocalManifestInDev(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testTarball(t, map[string]string{"bin/tool": "new"})
	sum := sha256.Sum256(body)
	remoteManifestHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/adapter.tgz":
			_, _ = w.Write(body)
		case "/manifest.json":
			remoteManifestHit = true
			_, _ = fmt.Fprintf(w, `{"adapters":{"claude":{"version":"0.9.0","assets":{"%s":{"url":"%s","sha256":"%064x","binary":"old"}}}}}`, platform, serverURL(r, "/old.tgz"), 1)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "dist", "acp-adapters.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(fmt.Sprintf(`{"adapters":{"claude":{"version":"1.2.3","assets":{"%s":{"url":"%s","sha256":"%x","binary":"bin/tool"}}}}}`, platform, server.URL+"/adapter.tgz", sum)), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewForTest(root, server.URL+"/manifest.json", server.Client())
	manager.localManifestPath = manifestPath
	launch, err := manager.ResolveAdapter(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if remoteManifestHit {
		t.Fatal("remote latest manifest was fetched despite local dev manifest")
	}
	if !strings.Contains(launch.Command, "1.2.3") {
		t.Fatalf("launch command = %q", launch.Command)
	}
}

func TestFetchManifestRejectsStaleCacheWhenLocalManifestIsInvalid(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "dist", "acp-adapters.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewForTest(root, "", nil)
	manager.localManifestPath = manifestPath
	manager.cacheManifest(manifest{Adapters: map[string]manifestAdapter{
		"claude": {Version: "cached", Assets: map[string]manifestAsset{}},
	}})

	if _, err := manager.fetchManifest(context.Background()); err == nil {
		t.Fatal("expected invalid local manifest to reject cache fallback")
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

func TestExtractZipRejectsUnsafeArchivePath(t *testing.T) {
	for _, name := range []string{"../kimi", `..\kimi`, "C:/kimi"} {
		err := extractArchive(testZip(t, map[string]string{name: "bad"}), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "unsafe path") {
			t.Fatalf("unsafe ZIP path %q error = %v", name, err)
		}
	}
}

func TestStatusReportsMissingBeforeManifestDownload(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	status := New(t.TempDir(), "dev").Status("codex")
	if status.State != StateMissing || status.Platform != platform {
		t.Fatalf("status = %#v", status)
	}
}

func TestManifestURLForVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
		cache   string
	}{
		{version: "", want: releasesURL + "/latest/download/acp-adapters.json", cache: "latest"},
		{version: "dev", want: releasesURL + "/latest/download/acp-adapters.json", cache: "latest"},
		{version: "0.0.51", want: releasesURL + "/download/v0.0.51/acp-adapters.json", cache: "v0.0.51"},
		{version: "v0.0.51", want: releasesURL + "/download/v0.0.51/acp-adapters.json", cache: "v0.0.51"},
	}
	for _, tt := range tests {
		if got := manifestURLForVersion(tt.version); got != tt.want {
			t.Fatalf("manifestURLForVersion(%q) = %q, want %q", tt.version, got, tt.want)
		}
		if got := manifestCacheNameForVersion(tt.version); got != tt.cache {
			t.Fatalf("manifestCacheNameForVersion(%q) = %q, want %q", tt.version, got, tt.cache)
		}
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
	return testTarballMode(t, files, 0o755)
}

func testTarballMode(t *testing.T, files map[string]string, mode int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(content))}); err != nil {
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

func testZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		file, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
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
