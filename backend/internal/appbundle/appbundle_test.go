package appbundle

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTestZip(t *testing.T, entries []struct {
	name string
	data []byte
	mode fs.FileMode
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{
			Name:   e.name,
			Method: zip.Deflate,
		}
		if e.mode != 0 {
			hdr.SetMode(e.mode)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPackFilesDeterministicSHAAndAudit(t *testing.T) {
	m := Manifest{
		Format:       Format,
		Title:        "Budget Tracker",
		Description:  "Interactive finance dashboard",
		Kind:         KindArtifact,
		Entry:        "index.html",
		Capabilities: []string{CapAIComplete, CapStoragePersonal, CapStoragePersonal},
		AllowedTools: []string{"crm_search", "crm_search", "tasks_list"},
		Icon:         &IconSpec{Kind: "emoji", Value: "📊"},
	}
	html := `<!doctype html><html><head>
<script src="https://esm.sh/react@19.2.0"></script>
<script src="https://evil.example.com/beacon.js"></script>
</head><body><script>navigator.serviceWorker?.register('/sw.js'); console.log('/Users/alice/secret.txt');</script></body></html>`

	b1, err := PackFiles(m, map[string][]byte{
		"index.html": []byte(html),
		"styles.css": []byte("body { color: #111; }"),
	})
	if err != nil {
		t.Fatalf("PackFiles b1: %v", err)
	}

	m2 := m
	m2.Capabilities = []string{CapStoragePersonal, CapAIComplete}
	m2.AllowedTools = []string{"tasks_list", "crm_search"}
	b2, err := PackFiles(m2, map[string][]byte{
		"styles.css": []byte("body { color: #111; }"),
		"index.html": []byte(html),
	})
	if err != nil {
		t.Fatalf("PackFiles b2: %v", err)
	}
	if b1.BundleSHA256 != b2.BundleSHA256 {
		t.Fatalf("expected deterministic bundle_sha256, got %s vs %s", b1.BundleSHA256, b2.BundleSHA256)
	}
	if len(b1.ExternalURLs) != 2 {
		t.Fatalf("expected 2 audited external URLs, got %+v", b1.ExternalURLs)
	}
	var sawCDN, sawEvil bool
	for _, u := range b1.ExternalURLs {
		if u.Host == "esm.sh" && u.Allowlisted {
			sawCDN = true
		}
		if u.Host == "evil.example.com" && !u.Allowlisted {
			sawEvil = true
		}
	}
	if !sawCDN || !sawEvil {
		t.Fatalf("unexpected CDN allowlist classification: %+v", b1.ExternalURLs)
	}
	joinedWarn := strings.Join(b1.Warnings, "\n")
	if !strings.Contains(joinedWarn, "evil.example.com") ||
		!strings.Contains(joinedWarn, "local filesystem path") ||
		!strings.Contains(joinedWarn, "serviceWorker") {
		t.Fatalf("expected preflight warnings for external host, local path, and serviceWorker, got %v", b1.Warnings)
	}
}

func TestHostileBundleGuards(t *testing.T) {
	validManifest := []byte(`{"format":"jaz-app/1","title":"Test","kind":"artifact","entry":"index.html"}`)

	t.Run("RejectPathTraversalInZip", func(t *testing.T) {
		z := makeTestZip(t, []struct {
			name string
			data []byte
			mode fs.FileMode
		}{
			{name: "manifest.json", data: validManifest},
			{name: "index.html", data: []byte("<p>ok</p>")},
			{name: "../escape.txt", data: []byte("boom")},
		})
		if _, err := PackZipBytes(z); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath, got %v", err)
		}
	})

	t.Run("RejectReservedHostPrefix", func(t *testing.T) {
		z := makeTestZip(t, []struct {
			name string
			data []byte
			mode fs.FileMode
		}{
			{name: "manifest.json", data: validManifest},
			{name: "index.html", data: []byte("<p>ok</p>")},
			{name: ".jaz/viewer.js", data: []byte("alert(1)")},
		})
		if _, err := PackZipBytes(z); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath for .jaz/, got %v", err)
		}
	})

	t.Run("RejectSymlinkInZip", func(t *testing.T) {
		z := makeTestZip(t, []struct {
			name string
			data []byte
			mode fs.FileMode
		}{
			{name: "manifest.json", data: validManifest},
			{name: "index.html", data: []byte("<p>ok</p>")},
			{name: "link.txt", data: []byte("/etc/passwd"), mode: fs.ModeSymlink | 0o777},
		})
		if _, err := PackZipBytes(z); !errors.Is(err, ErrSymlinkBlocked) {
			t.Fatalf("expected ErrSymlinkBlocked, got %v", err)
		}
	})

	t.Run("RejectCaseCollision", func(t *testing.T) {
		m := Manifest{Format: Format, Title: "Case", Kind: KindArtifact, Entry: "index.html"}
		_, err := PackFiles(m, map[string][]byte{
			"index.html": []byte("<p>a</p>"),
			"Index.HTML": []byte("<p>b</p>"),
		})
		if !errors.Is(err, ErrDuplicatePath) {
			t.Fatalf("expected ErrDuplicatePath, got %v", err)
		}
	})

	t.Run("RejectPerFileAndTotalSizeCaps", func(t *testing.T) {
		m := Manifest{Format: Format, Title: "Big", Kind: KindArtifact, Entry: "index.html"}
		oversized := bytes.Repeat([]byte("A"), int(MaxFileBytes)+1)
		if _, err := PackFiles(m, map[string][]byte{
			"index.html": []byte("<p>ok</p>"),
			"big.bin":    oversized,
		}); !errors.Is(err, ErrFileTooLarge) {
			t.Fatalf("expected ErrFileTooLarge, got %v", err)
		}

		z := makeTestZip(t, []struct {
			name string
			data []byte
			mode fs.FileMode
		}{
			{name: "manifest.json", data: validManifest},
			{name: "index.html", data: []byte("<p>ok</p>")},
			{name: "bomb.txt", data: bytes.Repeat([]byte("0"), int(MaxFileBytes)+1024)},
		})
		if _, err := PackZipBytes(z); !errors.Is(err, ErrFileTooLarge) {
			t.Fatalf("expected ErrFileTooLarge on zip bomb, got %v", err)
		}

		chunk := bytes.Repeat([]byte("B"), 9<<20)
		if _, err := PackFiles(m, map[string][]byte{
			"index.html": []byte("<p>ok</p>"),
			"a.bin":      chunk,
			"b.bin":      chunk,
			"c.bin":      chunk,
		}); !errors.Is(err, ErrBundleTooLarge) {
			t.Fatalf("expected ErrBundleTooLarge, got %v", err)
		}
	})

	t.Run("RejectTooManyFiles", func(t *testing.T) {
		m := Manifest{Format: Format, Title: "Many", Kind: KindArtifact, Entry: "index.html"}
		many := map[string][]byte{"index.html": []byte("<p>ok</p>")}
		for i := 0; i < MaxFileCount; i++ {
			many[filepath.ToSlash(filepath.Join("d", strings.Repeat("x", 1)+string(rune('a'+(i/26)%26))+string(rune('a'+i%26))+".txt"))] = []byte("x")
		}
		if _, err := PackFiles(m, many); !errors.Is(err, ErrTooManyFiles) {
			t.Fatalf("expected ErrTooManyFiles, got %v", err)
		}
	})

	t.Run("RejectSymlinkInPackDir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), validManifest, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>ok</p>"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/etc/hosts", filepath.Join(dir, "escape.txt")); err != nil {
			t.Fatal(err)
		}
		if _, err := PackDir(dir); !errors.Is(err, ErrSymlinkBlocked) {
			t.Fatalf("expected ErrSymlinkBlocked from PackDir, got %v", err)
		}
	})

	t.Run("RejectInvalidCapabilitiesAndWildcardTools", func(t *testing.T) {
		m := Manifest{
			Format:       Format,
			Title:        "BadCap",
			Kind:         KindArtifact,
			Entry:        "index.html",
			Capabilities: []string{"root:shell"},
		}
		if _, err := PackFiles(m, map[string][]byte{"index.html": []byte("<p>x</p>")}); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("expected ErrInvalidManifest for invalid cap, got %v", err)
		}
		m.Capabilities = []string{CapToolsCall}
		m.AllowedTools = []string{"*"}
		if _, err := PackFiles(m, map[string][]byte{"index.html": []byte("<p>x</p>")}); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("expected ErrInvalidManifest for wildcard tool, got %v", err)
		}
	})
}
