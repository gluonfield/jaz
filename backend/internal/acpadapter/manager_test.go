package acpadapter

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	server := archiveServer(t, "/claude.tgz", body)

	root := t.TempDir()
	incomplete := filepath.Join(root, "acp", "managed", "adapters", "claude", "1.2.3", platform, "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(incomplete), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(incomplete, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := testManager(t, root, server, "claude", manifestAsset{URL: server.URL + "/claude.tgz", SHA256: sha256Hex(body), Binary: "bin/tool", Env: map[string]string{"CLAUDE_CODE_EXECUTABLE": "bin/claude"}})
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
	binary := "kimi"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	body := testZip(t, map[string]string{binary: "ok"})
	server := archiveServer(t, "/kimi.zip", body)
	manager := testManager(t, t.TempDir(), server, "kimi", manifestAsset{URL: server.URL + "/kimi.zip", SHA256: sha256Hex(body), Binary: binary})
	launch, err := manager.ResolveAdapter(context.Background(), "kimi")
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
	body := testTarball(t, map[string]string{"bin/tool": strings.Repeat("ok", 1024)})
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

	manager := testManager(t, t.TempDir(), server, "claude", manifestAsset{URL: server.URL + "/claude.tgz", SHA256: sha256Hex(body), Binary: "bin/tool"})
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

func TestResolveAdapterRejectsBadChecksum(t *testing.T) {
	server := archiveServer(t, "/codex.tgz", testTarball(t, map[string]string{"tool": "ok"}))
	manager := testManager(t, t.TempDir(), server, "codex", manifestAsset{URL: server.URL + "/codex.tgz", SHA256: strings.Repeat("0", 64), Binary: "tool"})
	if _, err := manager.ResolveAdapter(context.Background(), "codex"); err == nil {
		t.Fatal("expected checksum error")
	}
	status := manager.Status("codex")
	if status.State != StateFailed || status.Message == "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestResolveAdapterRejectsUnsafeArchivePath(t *testing.T) {
	body := testTarball(t, map[string]string{"../tool": "bad", "tool": "ok"})
	server := archiveServer(t, "/codex.tgz", body)
	manager := testManager(t, t.TempDir(), server, "codex", manifestAsset{URL: server.URL + "/codex.tgz", SHA256: sha256Hex(body), Binary: "tool"})
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

func TestPrepareRecordsFailedDownload(t *testing.T) {
	server := archiveServer(t, "/codex.tgz", nil)
	manager := testManager(t, t.TempDir(), server, "codex", manifestAsset{URL: server.URL + "/codex.tgz", SHA256: strings.Repeat("0", 64), Binary: "tool"})
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
	server := archiveServer(t, "/codex.tgz", nil)
	manager := testManager(t, root, server, "codex", manifestAsset{URL: server.URL + "/codex.tgz", SHA256: strings.Repeat("0", 64), Binary: "tool"})
	launch, err := manager.ResolveAdapter(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	if launch.Command != path {
		t.Fatalf("command = %q, want %q", launch.Command, path)
	}
}

func TestStatusReportsPinnedAdapterInstalledOnStart(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	if status := New(root).Status("codex"); status.State != StateMissing || status.Platform != platform {
		t.Fatalf("status before install = %#v", status)
	}
	spec, err := New(root).resolveSpec("codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range spec.executables() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("ok"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	status := New(root).Status("codex")
	if status.State != StateReady || status.Path != spec.Command || status.Version != spec.Version {
		t.Fatalf("status = %#v", status)
	}
}

func testManager(t *testing.T, root string, server *httptest.Server, name string, asset manifestAsset) *Manager {
	t.Helper()
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	manager := New(root)
	manager.client = server.Client()
	manager.manifest = manifest{Adapters: map[string]manifestAdapter{
		name: {Version: "1.2.3", Assets: map[string]manifestAsset{platform: asset}},
	}}
	return manager
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func archiveServer(t *testing.T, path string, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || body == nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
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
