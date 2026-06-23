package acpadapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

func TestInstallNPMBinaryDownloadsVerifiesAndExtractsExpectedBinary(t *testing.T) {
	body := testTarball(t, "package/bin/tool", "ok")
	sum := sha512.Sum512(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/@scope%2fpkg/1.2.3", "/@scope/pkg/1.2.3":
			_, _ = fmt.Fprintf(w, `{"dist":{"tarball":"%s","integrity":"sha512-%s"}}`, serverURL(r, "/pkg.tgz"), base64.StdEncoding.EncodeToString(sum[:]))
		case "/pkg.tgz":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL, server.Client())
	dst := filepath.Join(t.TempDir(), "tool")
	if err := manager.installNPMBinary(context.Background(), "@scope/pkg", "1.2.3", "package/bin/tool", dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Fatalf("binary = %q", got)
	}
}

func TestInstallNPMBinaryRejectsBadIntegrity(t *testing.T) {
	body := testTarball(t, "package/bin/tool", "ok")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/@scope%2fpkg/1.2.3", "/@scope/pkg/1.2.3":
			_, _ = fmt.Fprintf(w, `{"dist":{"tarball":"%s","integrity":"sha512-%s"}}`, serverURL(r, "/pkg.tgz"), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, sha512.Size)))
		case "/pkg.tgz":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL, server.Client())
	err := manager.installNPMBinary(context.Background(), "@scope/pkg", "1.2.3", "package/bin/tool", filepath.Join(t.TempDir(), "tool"))
	if err == nil {
		t.Fatal("expected integrity error")
	}
}

func TestStatusReportsMissingThenReadyCodexBinary(t *testing.T) {
	root := t.TempDir()
	spec, err := codexSpec(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	manager := New(root)
	status := manager.Status("codex")
	if status.State != StateMissing || status.Version != acp.CodexACPVersion || status.Platform != spec.Platform {
		t.Fatalf("missing status = %#v", status)
	}
	path := manager.adapterPath("codex", acp.CodexACPVersion, spec.Platform, spec.BinaryName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	status = manager.Status("codex")
	if status.State != StateReady || status.Path != path {
		t.Fatalf("ready status = %#v", status)
	}
}

func TestPrepareRecordsFailedCodexDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewForTest(t.TempDir(), server.URL, server.Client())
	if err := manager.Prepare(context.Background(), "codex"); err == nil {
		t.Fatal("expected prepare error")
	}
	status := manager.Status("codex")
	if status.State != StateFailed || status.Message == "" {
		t.Fatalf("failed status = %#v", status)
	}
}

func TestResolveAdapterReusesInstalledCodexBinary(t *testing.T) {
	root := t.TempDir()
	spec, err := codexSpec(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	manager := New(root)
	path := manager.adapterPath("codex", acp.CodexACPVersion, spec.Platform, spec.BinaryName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	launch, err := manager.ResolveAdapter(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	if launch.Command != path {
		t.Fatalf("command = %q, want %q", launch.Command, path)
	}
}

func testTarball(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
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
