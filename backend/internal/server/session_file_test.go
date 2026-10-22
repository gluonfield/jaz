package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/filepathx"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionFileRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "src", "previewWebview.ts")
	if err := os.WriteFile(file, []byte("export type Preview = string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       "file-session",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	noCwd, err := store.CreateSession(storage.CreateSession{Slug: "no-cwd"})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()

	get := func(path string, want int) sessionFileResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("GET %s = %d, want %d; body = %s", path, res.Code, want, res.Body.String())
		}
		var got sessionFileResponse
		if want == http.StatusOK {
			if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
		}
		return got
	}

	relative := get("/v1/sessions/"+session.ID+"/file?path=src/previewWebview.ts", http.StatusOK)
	if relative.Path != file || relative.RelativePath != "src/previewWebview.ts" || !strings.Contains(relative.Content, "Preview") {
		t.Fatalf("relative read = %#v", relative)
	}

	absolute := get("/v1/sessions/"+session.ID+"/file?path="+url.QueryEscape(file), http.StatusOK)
	if absolute.Path != file || absolute.Content != relative.Content {
		t.Fatalf("absolute read = %#v", absolute)
	}

	fileURL := get("/v1/sessions/"+session.ID+"/file?path="+url.QueryEscape(filepathx.FileURI(file)), http.StatusOK)
	if fileURL.Path != file || fileURL.Content != relative.Content {
		t.Fatalf("file URL read = %#v", fileURL)
	}

	pixels := image.NewNRGBA(image.Rect(0, 0, 1024, 512))
	if _, err := rand.New(rand.NewSource(1)).Read(pixels.Pix); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	if encoded.Len() <= sessionFileReadLimit {
		t.Fatal("image must exceed the text preview limit")
	}
	rawHandler := (&Server{Store: store, AuthKey: "image-key"}).Handler()
	for _, asset := range []struct {
		name        string
		contentType string
		content     []byte
	}{
		{"paper.pdf", "application/pdf", []byte("%PDF-1.7\nbody")},
		{"chart #1.png", "image/png", encoded.Bytes()},
		{"chart #1.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><circle cx="5" cy="5" r="4"/></svg>`)},
	} {
		t.Run(asset.name, func(t *testing.T) {
			file := filepath.Join(dir, asset.name)
			if err := os.WriteFile(file, asset.content, 0o644); err != nil {
				t.Fatal(err)
			}
			for _, reference := range []string{asset.name, file, filepathx.FileURI(file)} {
				for _, key := range []string{"", "wrong-key", "image-key"} {
					params := url.Values{"path": {reference}, "raw": {"1"}, "key": {key}}
					req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/file?"+params.Encode(), nil)
					res := httptest.NewRecorder()
					rawHandler.ServeHTTP(res, req)
					if key != "image-key" {
						if res.Code != http.StatusUnauthorized {
							t.Fatalf("unauthenticated raw file status = %d", res.Code)
						}
						continue
					}
					if res.Code != http.StatusOK || !bytes.Equal(res.Body.Bytes(), asset.content) {
						t.Fatalf("raw file %q: status = %d, received %d bytes, want %d", reference, res.Code, res.Body.Len(), len(asset.content))
					}
					if got := res.Header().Get("Content-Type"); got != asset.contentType {
						t.Fatalf("Content-Type = %q, want %q", got, asset.contentType)
					}
					if got := res.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") || !strings.Contains(got, asset.name) {
						t.Fatalf("Content-Disposition = %q", got)
					}
					if got := res.Header().Get("X-Content-Type-Options"); got != "nosniff" {
						t.Fatalf("X-Content-Type-Options = %q", got)
					}
					if got := res.Header().Get("Cache-Control"); got != "no-store" {
						t.Fatalf("mutable file Cache-Control = %q, want no-store", got)
					}
				}
			}
		})
	}

	tempFile := filepath.Join(t.TempDir(), "agent-output.txt")
	if err := os.WriteFile(tempFile, []byte("from temp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tempRead := get("/v1/sessions/"+session.ID+"/file?path="+url.QueryEscape(tempFile), http.StatusOK)
	if tempRead.Path != tempFile || tempRead.RelativePath != "" || tempRead.Content != "from temp\n" {
		t.Fatalf("temp read = %#v", tempRead)
	}

	if runtime.GOOS != "windows" && filepath.Clean(os.TempDir()) != "/tmp" {
		tmpDir, err := os.MkdirTemp("/tmp", "jaz-session-file-")
		if err == nil {
			defer os.RemoveAll(tmpDir)
			tmpFile := filepath.Join(tmpDir, "agent-output.txt")
			if err := os.WriteFile(tmpFile, []byte("from /tmp\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			tmpRead := get("/v1/sessions/"+session.ID+"/file?path="+url.QueryEscape(tmpFile), http.StatusOK)
			if tmpRead.Path != tmpFile || tmpRead.Content != "from /tmp\n" {
				t.Fatalf("/tmp read = %#v", tmpRead)
			}
		}
	}

	outsideDir, err := os.MkdirTemp("", "jaz-outside-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outsideDir)
	outsideFile := filepath.Join(outsideDir, "elsewhere.txt")
	if err := os.WriteFile(outsideFile, []byte("from outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideRead := get("/v1/sessions/"+session.ID+"/file?path="+url.QueryEscape(outsideFile), http.StatusOK)
	if outsideRead.Path != outsideFile || outsideRead.RelativePath != "" || outsideRead.Content != "from outside\n" {
		t.Fatalf("outside read = %#v", outsideRead)
	}

	missing := filepath.Join(string(filepath.Separator), "definitely-not-a-jaz-session-file")
	get("/v1/sessions/"+session.ID+"/file?path="+url.QueryEscape(missing), http.StatusNotFound)
	get("/v1/sessions/"+noCwd.ID+"/file?path=src/previewWebview.ts", http.StatusBadRequest)
}

func TestSessionFileWrite(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "part.stl")
	if err := os.WriteFile(file, []byte("solid a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "write", RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: dir}})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()
	fileURL := "/v1/sessions/" + session.ID + "/file?path=part.stl&raw=1"
	send := func(method, body, ifMatch string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, fileURL, strings.NewReader(body))
		if ifMatch != "" {
			req.Header.Set("If-Match", ifMatch)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// Each version differs in size, as coarse file timestamps can repeat
	// within a test.
	read := send(http.MethodGet, "", "")
	original := read.Header().Get("ETag")
	if read.Code != http.StatusOK || original == "" {
		t.Fatalf("raw read = %d with ETag %q", read.Code, original)
	}
	saved := send(http.MethodPut, "solid bb\n", original)
	if saved.Code != http.StatusNoContent || saved.Header().Get("ETag") == original {
		t.Fatalf("save = %d with ETag %q", saved.Code, saved.Header().Get("ETag"))
	}
	if data, _ := os.ReadFile(file); string(data) != "solid bb\n" {
		t.Fatalf("file = %q", data)
	}
	if again := send(http.MethodGet, "", ""); again.Header().Get("ETag") != saved.Header().Get("ETag") {
		t.Fatalf("read after save has ETag %q, want %q", again.Header().Get("ETag"), saved.Header().Get("ETag"))
	}
	stale := send(http.MethodPut, "solid ccc\n", original)
	if stale.Code != http.StatusPreconditionFailed || stale.Header().Get("ETag") != saved.Header().Get("ETag") {
		t.Fatalf("a save against an old version = %d with ETag %q", stale.Code, stale.Header().Get("ETag"))
	}
	if data, _ := os.ReadFile(file); string(data) != "solid bb\n" {
		t.Fatalf("a refused save overwrote the file: %q", data)
	}
	if blind := send(http.MethodPut, "solid dddd\n", ""); blind.Code != http.StatusNoContent {
		t.Fatalf("a save without If-Match = %d", blind.Code)
	}
	big := send(http.MethodPut, strings.Repeat("x", sessionFileWriteLimit+1), "")
	if big.Code != http.StatusRequestEntityTooLarge || !strings.Contains(big.Body.String(), `"max_bytes":10485760`) {
		t.Fatalf("an oversized save = %d %s", big.Code, big.Body.String())
	}
	if data, _ := os.ReadFile(file); string(data) != "solid dddd\n" {
		t.Fatalf("an oversized save changed the file: %q", data)
	}
}
