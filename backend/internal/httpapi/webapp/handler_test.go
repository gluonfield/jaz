package webapp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlerServesWebApp(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := Handler{Dir: dir}

	tests := []struct {
		path   string
		status int
		body   string
	}{
		{path: "/", status: http.StatusOK, body: "index"},
		{path: "/sessions/abc", status: http.StatusOK, body: "index"},
		{path: "/assets/app.js", status: http.StatusOK, body: "asset"},
		{path: "/assets/missing.js", status: http.StatusNotFound},
		{path: "/v1/sessions", status: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			if res.Code != tt.status {
				t.Fatalf("status = %d, want %d; body = %s", res.Code, tt.status, res.Body.String())
			}
			if tt.body != "" && !strings.Contains(res.Body.String(), tt.body) {
				t.Fatalf("body = %q, want %q", res.Body.String(), tt.body)
			}
		})
	}
}
