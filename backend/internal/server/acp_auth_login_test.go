package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestACPAuthLoginRunsCodexWithoutHome(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "codex"), `#!/bin/sh
printf 'home=%s\n' "$HOME"
printf 'codex_home=%s\n' "$CODEX_HOME"
`)
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store, Root: root}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/codex/auth/login", strings.NewReader(`{"auth":{"mode":"jaz_profile"}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", res.Code, res.Body.String())
	}
	var started acpAuthLoginResponse
	if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}

	done := waitForACPAuthLogin(t, handler, started.ID)
	if done.Status != "succeeded" {
		t.Fatalf("login status = %#v", done)
	}
	if strings.Contains(done.Output, "home="+home) {
		t.Fatalf("codex login inherited HOME: %q", done.Output)
	}
	if !strings.Contains(done.Output, "codex_home="+filepath.Join(root, "acp", "codex-home")) {
		t.Fatalf("codex login output = %q", done.Output)
	}
}

func TestACPAuthLoginRunsGrokWithExistingHome(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "grok"), `#!/bin/sh
printf 'home=%s\n' "$HOME"
`)
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store, Root: root}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/grok/auth/login", strings.NewReader(`{"auth":{"mode":"existing_cli"}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", res.Code, res.Body.String())
	}
	var started acpAuthLoginResponse
	if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}

	done := waitForACPAuthLogin(t, handler, started.ID)
	if done.Status != "succeeded" {
		t.Fatalf("login status = %#v", done)
	}
	if !strings.Contains(done.Output, "home="+home) {
		t.Fatalf("grok login did not inherit existing HOME: %q", done.Output)
	}
}

func TestACPAuthLoginAllowsEmptyBody(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "grok"), `#!/bin/sh
printf ok
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store, Root: root}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/grok/auth/login", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", res.Code, res.Body.String())
	}
}

func waitForACPAuthLogin(t *testing.T, handler http.Handler, id string) acpAuthLoginResponse {
	t.Helper()
	var got acpAuthLoginResponse
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/v1/acp/auth-logins/"+id, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("get status = %d, body = %s", res.Code, res.Body.String())
		}
		if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Status != "running" {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("login %s did not finish: %#v", id, got)
	return got
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
