package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	outside := filepath.Join(string(filepath.Separator), "definitely-not-a-jaz-session-file")
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       "file-session",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeNative, Cwd: dir},
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

	get("/v1/sessions/"+session.ID+"/file?path="+url.QueryEscape(outside), http.StatusBadRequest)
	get("/v1/sessions/"+noCwd.ID+"/file?path=src/previewWebview.ts", http.StatusBadRequest)
}
