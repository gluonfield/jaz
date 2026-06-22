package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

func TestACPAuthLoginRunsCodexWithoutHome(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "codex"), `#!/bin/sh
printf 'home=%s\n' "$HOME"
printf 'codex_home=%s\n' "$CODEX_HOME"
printf '{}' > "$CODEX_HOME/auth.json"
`, `@echo off
echo home=%HOME%
echo codex_home=%CODEX_HOME%
echo {} > "%CODEX_HOME%\auth.json"
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
	assertCodexLoginUsesFileCredentials(t, root)
}

// Real codex aborts ("CODEX_HOME points to … but that path does not exist")
// before it can sign in, so the login runner must create the profile dir first.
func TestACPAuthLoginCreatesCodexProfileDir(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "codex"), `#!/bin/sh
if [ ! -d "$CODEX_HOME" ]; then
  echo "CODEX_HOME points to \"$CODEX_HOME\", but that path does not exist" >&2
  exit 1
fi
printf '{}' > "$CODEX_HOME/auth.json"
printf ok
`, `@echo off
if not exist "%CODEX_HOME%\" (
  echo CODEX_HOME points to "%CODEX_HOME%", but that path does not exist 1>&2
  exit /b 1
)
echo {} > "%CODEX_HOME%\auth.json"
echo ok
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
		t.Fatalf("login status = %#v (profile dir not created before login?)", done)
	}
	if _, err := os.Stat(filepath.Join(root, "acp", "codex-home")); err != nil {
		t.Fatalf("codex profile dir not created: %v", err)
	}
	assertCodexLoginUsesFileCredentials(t, root)
}

func TestACPAuthLoginUsesJazProfileEvenWithExistingCodexAuth(t *testing.T) {
	home := t.TempDir()
	existing := filepath.Join(home, ".codex")
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existing, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "codex"), `#!/bin/sh
printf 'codex_home=%s\n' "$CODEX_HOME"
printf '{}' > "$CODEX_HOME/auth.json"
`, `@echo off
echo codex_home=%CODEX_HOME%
echo {} > "%CODEX_HOME%\auth.json"
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

	body, err := json.Marshal(map[string]any{"auth": map[string]string{"mode": "existing_cli", "path": existing}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/codex/auth/login", strings.NewReader(string(body)))
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
	want := filepath.Join(root, "acp", "codex-home")
	if !strings.Contains(done.Output, "codex_home="+want) || strings.Contains(done.Output, "codex_home="+existing) {
		t.Fatalf("codex login output = %q, want Jaz profile %s", done.Output, want)
	}
	assertCodexLoginUsesFileCredentials(t, root)
}

func TestACPAuthLoginFailsWhenCredentialNotSaved(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "codex"), `#!/bin/sh
printf ok
`, `@echo off
echo ok
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
	if done.Status != "failed" || !strings.Contains(done.Error, filepath.Join(root, "acp", "codex-home", "auth.json")) {
		t.Fatalf("login status = %#v, want failed missing saved credential", done)
	}
	assertCodexLoginUsesFileCredentials(t, root)
}

func TestACPAuthLoginRunsGrokWithExistingHome(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "grok"), `#!/bin/sh
printf 'home=%s\n' "$HOME"
mkdir -p "$HOME/.grok"
printf '{}' > "$HOME/.grok/auth.json"
`, `@echo off
echo home=%HOME%
mkdir "%HOME%\.grok"
echo {} > "%HOME%\.grok\auth.json"
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
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "grok"), `#!/bin/sh
mkdir -p "$HOME/.grok"
printf '{}' > "$HOME/.grok/auth.json"
printf ok
`, `@echo off
mkdir "%HOME%\.grok"
echo {} > "%HOME%\.grok\auth.json"
echo ok
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

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/grok/auth/login", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", res.Code, res.Body.String())
	}
}

func assertCodexLoginUsesFileCredentials(t *testing.T, root string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "acp", "codex-home", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `cli_auth_credentials_store = "file"`) {
		t.Fatalf("codex config = %q, want file credential store", string(data))
	}
}

func TestACPAuthLoginResponseExtractsDeviceAuthHints(t *testing.T) {
	job := &acpAuthLoginJob{
		ID:     "login_test",
		Agent:  "codex",
		Status: "running",
		Output: "Open \x1b[94mhttps://auth.openai.com/codex/device\x1b[0m\ncode \x1b[94mM17M-3K1Z5\x1b[0m\n",
	}
	res := job.response()
	if res.AuthURL != "https://auth.openai.com/codex/device" {
		t.Fatalf("auth url = %q", res.AuthURL)
	}
	if res.AuthCode != "M17M-3K1Z5" {
		t.Fatalf("auth code = %q", res.AuthCode)
	}
}

func TestACPAuthLoginTimeoutAsksForFreshCode(t *testing.T) {
	job := &acpAuthLoginJob{Status: "running"}
	job.finish(nil, context.DeadlineExceeded)
	if job.Status != "failed" {
		t.Fatalf("status = %q", job.Status)
	}
	if !strings.Contains(job.Error, "fresh code") {
		t.Fatalf("error = %q", job.Error)
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
