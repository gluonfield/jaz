package managedtool

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrepareDownloadsVerifiesAndInstallsAntigravityCLI(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := testAntigravityTarball(t, "ok")
	assetPath := "/agy.tar.gz"
	if runtime.GOOS == "windows" {
		body = []byte("ok")
		assetPath = "/agy.exe"
	}
	sum := sha512.Sum512(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifests/" + platform + ".json":
			_, _ = fmt.Fprintf(w, `{"version":"1.2.3","url":"%s","sha512":"%x"}`, serverURL(r, assetPath), sum)
		case assetPath:
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL, server.Client())
	if err := manager.Prepare(context.Background(), AntigravityCLI); err != nil {
		t.Fatal(err)
	}
	status := manager.Status(AntigravityCLI)
	if status.State != StateReady || status.Version != "1.2.3" || status.Platform != platform {
		t.Fatalf("status = %#v", status)
	}
	got, err := os.ReadFile(status.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Fatalf("installed binary = %q", got)
	}
}

func TestPrepareRejectsBadChecksum(t *testing.T) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	body := []byte("bad")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifests/" + platform + ".json":
			_, _ = fmt.Fprintf(w, `{"version":"1.2.3","url":"%s","sha512":"%0128x"}`, serverURL(r, "/agy"), 1)
		case "/agy":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL, server.Client())
	if err := manager.Prepare(context.Background(), AntigravityCLI); err == nil {
		t.Fatal("expected checksum error")
	}
	if status := manager.Status(AntigravityCLI); status.State != StateFailed {
		t.Fatalf("status = %#v, want failed", status)
	}
}

func TestStatusFindsExistingManagedTool(t *testing.T) {
	root := t.TempDir()
	path := ExecutablePath(root, AntigravityCLI)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if status := New(root).Status(AntigravityCLI); status.State != StateReady || status.Path != path {
		t.Fatalf("status = %#v", status)
	}
}

func testAntigravityTarball(t *testing.T, payload string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(payload)
	if err := tw.WriteHeader(&tar.Header{Name: "antigravity", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
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
