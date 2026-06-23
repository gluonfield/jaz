package main

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
	"strings"
	"testing"
)

func TestParseUpdateArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want updateArgs
	}{
		{name: "default latest", want: updateArgs{}},
		{name: "explicit latest", in: []string{"--latest"}, want: updateArgs{}},
		{name: "version with v", in: []string{"--version", "v0.0.46"}, want: updateArgs{Version: "v0.0.46"}},
		{name: "version without v", in: []string{"--version", "0.0.46"}, want: updateArgs{Version: "v0.0.46"}},
		{name: "help", in: []string{"-h"}, want: updateArgs{Help: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUpdateArgs(tt.in)
			if err != nil {
				t.Fatalf("parseUpdateArgs: %v", err)
			}
			if got != tt.want {
				t.Fatalf("args = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseUpdateArgsRejectsConflicts(t *testing.T) {
	if _, err := parseUpdateArgs([]string{"--latest", "--version", "v0.0.46"}); err == nil {
		t.Fatal("expected conflict error")
	}
	if _, err := parseUpdateArgs([]string{"extra"}); err == nil {
		t.Fatal("expected unexpected argument error")
	}
}

func TestUpdaterReplacesExecutable(t *testing.T) {
	archive := testBackendArchive(t, "new binary", 0755)
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download/v0.0.46/jaz-backend-linux-amd64.tar.gz":
			_, _ = w.Write(archive)
		case "/download/v0.0.46/jaz-backend-linux-amd64.tar.gz.sha256":
			fmt.Fprintf(w, "%x  jaz-backend-linux-amd64.tar.gz\n", sum)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exe := filepath.Join(t.TempDir(), "jaz")
	if err := os.WriteFile(exe, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	u := updater{BaseURL: server.URL, Executable: exe, GOOS: "linux", GOARCH: "amd64", Client: server.Client()}
	if err := u.update(context.Background(), updateArgs{Version: "v0.0.46"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Fatalf("binary = %q", got)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestUpdaterRejectsChecksumMismatch(t *testing.T) {
	archive := testBackendArchive(t, "new binary", 0755)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			fmt.Fprintln(w, strings.Repeat("0", sha256.Size*2))
		default:
			_, _ = w.Write(archive)
		}
	}))
	defer server.Close()

	exe := filepath.Join(t.TempDir(), "jaz")
	if err := os.WriteFile(exe, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	u := updater{BaseURL: server.URL, Executable: exe, GOOS: "darwin", GOARCH: "arm64", Client: server.Client()}
	err := u.update(context.Background(), updateArgs{})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old binary" {
		t.Fatalf("binary changed after checksum failure: %q", got)
	}
}

func TestBackendAssetNameRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := backendAssetName("windows", "amd64"); err == nil {
		t.Fatal("expected unsupported platform error")
	}
	if _, err := backendAssetName("linux", "386"); err == nil {
		t.Fatal("expected unsupported arch error")
	}
}

func testBackendArchive(t *testing.T, content string, mode int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	data := []byte(content)
	if err := tw.WriteHeader(&tar.Header{Name: "jaz", Mode: mode, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
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
