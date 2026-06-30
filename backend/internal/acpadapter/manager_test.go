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
	"strings"
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

func TestResolveAdapterPrefersLocalAssetSpecInDev(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testTarball(t, map[string]string{"bin/tool": "new"})
	sum := sha256.Sum256(body)
	staleManifestHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/example/adapter/releases/tags/v1.2.3":
			_, _ = fmt.Fprintf(w, `{"assets":[{"name":"adapter-1.2.3-%s.tar.gz","browser_download_url":"%s","digest":"sha256:%x"}]}`, platform, serverURL(r, "/adapter.tgz"), sum)
		case "/adapter.tgz":
			_, _ = w.Write(body)
		case "/manifest.json":
			staleManifestHit = true
			_, _ = fmt.Fprintf(w, `{"adapters":{"claude":{"version":"0.9.0","assets":{"%s":{"url":"%s","sha256":"%064x","binary":"old"}}}}}`, platform, serverURL(r, "/old.tgz"), 1)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	specPath := filepath.Join(root, "acp-adapter-assets.json")
	if err := os.WriteFile(specPath, []byte(fmt.Sprintf(`{"adapters":{"claude":{"repo":"example/adapter","tag":"v1.2.3","version":"1.2.3","assets":{"%s":{"name":"adapter-1.2.3-%s.tar.gz","binary":"bin/tool"}}}}}`, platform, platform)), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewForTest(root, server.URL+"/manifest.json", server.Client())
	manager.assetSpecPath = specPath
	manager.githubAPIURL = server.URL + "/repos"
	launch, err := manager.ResolveAdapter(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if staleManifestHit {
		t.Fatal("stale release manifest was fetched despite local asset spec")
	}
	if !strings.Contains(launch.Command, "1.2.3") {
		t.Fatalf("launch command = %q", launch.Command)
	}
	status := manager.Status("claude")
	if status.Version != "1.2.3" || status.State != StateReady {
		t.Fatalf("status = %#v", status)
	}
}

func TestFetchManifestRejectsStaleCacheWhenLocalAssetSpecFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusBadGateway)
	}))
	defer server.Close()

	root := t.TempDir()
	specPath := filepath.Join(root, "acp-adapter-assets.json")
	if err := os.WriteFile(specPath, []byte(`{"adapters":{"claude":{"repo":"example/adapter","tag":"v1.2.3","version":"1.2.3","assets":{"darwin-arm64":{"name":"adapter-1.2.3-darwin-arm64.tar.gz","binary":"bin/tool"}}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewForTest(root, "", server.Client())
	manager.assetSpecPath = specPath
	manager.githubAPIURL = server.URL + "/repos"
	manager.cacheManifest(manifest{Adapters: map[string]manifestAdapter{
		"claude": {Version: "cached", Assets: map[string]manifestAsset{}},
	}})

	if _, err := manager.fetchManifest(context.Background()); err == nil {
		t.Fatal("expected stale cache to be rejected")
	}
}

func TestFetchManifestFallsBackToMatchingCacheWhenLocalAssetSpecFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusBadGateway)
	}))
	defer server.Close()

	root := t.TempDir()
	specPath := filepath.Join(root, "acp-adapter-assets.json")
	if err := os.WriteFile(specPath, []byte(`{"adapters":{"claude":{"repo":"example/adapter","tag":"v1.2.3","version":"1.2.3","assets":{"darwin-arm64":{"name":"adapter-1.2.3-darwin-arm64.tar.gz","binary":"bin/tool"}}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewForTest(root, "", server.Client())
	manager.assetSpecPath = specPath
	manager.githubAPIURL = server.URL + "/repos"
	manager.cacheManifest(manifest{Adapters: map[string]manifestAdapter{
		"claude": {Version: "1.2.3", Assets: map[string]manifestAsset{}},
	}})

	got, err := manager.fetchManifest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Adapters["claude"].Version != "1.2.3" {
		t.Fatalf("manifest = %#v", got)
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
