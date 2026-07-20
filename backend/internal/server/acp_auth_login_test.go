package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

func TestACPAuthLoginEnvPreservesHostProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("HTTPS_PROXY", "http://secure-proxy.example:8443")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")

	env := strings.Join(acpAuthLoginEnv(acp.AgentLoginInvocation{}), "\n")
	for _, want := range []string{
		"HTTP_PROXY=http://proxy.example:8080",
		"HTTPS_PROXY=http://secure-proxy.example:8443",
		"NO_PROXY=localhost,127.0.0.1",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("login environment missing %q: %s", want, env)
		}
	}
}

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
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

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
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

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

// A headless/remote login can't capture an OAuth redirect, so the CLI blocks
// reading a code from stdin; the relay endpoint must hand it back.
func TestACPAuthLoginRelaysPastedCode(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "codex"), `#!/bin/sh
echo "visit https://auth.example/login"
read code
echo "received=$code"
printf '{}' > "$CODEX_HOME/auth.json"
`, `@echo off
echo visit https://auth.example/login
set /p code=
echo received=%code%
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
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

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

	// The mock blocks on `read`; the pipe buffers the code until it gets there.
	in := httptest.NewRequest(http.MethodPost, "/v1/acp/auth-logins/"+started.ID+"/input", strings.NewReader(`{"input":"PASTE-CODE-9"}`))
	in.Header.Set("Content-Type", "application/json")
	inRes := httptest.NewRecorder()
	handler.ServeHTTP(inRes, in)
	if inRes.Code != http.StatusOK {
		t.Fatalf("input status = %d, body = %s", inRes.Code, inRes.Body.String())
	}

	done := waitForACPAuthLogin(t, handler, started.ID)
	if done.Status != "succeeded" {
		t.Fatalf("login status = %#v", done)
	}
	if !strings.Contains(done.Output, "received=PASTE-CODE-9") {
		t.Fatalf("login did not receive pasted code: %q", done.Output)
	}

	// Once the process has exited, the relay rejects further input.
	late := httptest.NewRequest(http.MethodPost, "/v1/acp/auth-logins/"+started.ID+"/input", strings.NewReader(`{"input":"x"}`))
	late.Header.Set("Content-Type", "application/json")
	lateRes := httptest.NewRecorder()
	handler.ServeHTTP(lateRes, late)
	if lateRes.Code != http.StatusConflict {
		t.Fatalf("late input status = %d, want 409 (body %s)", lateRes.Code, lateRes.Body.String())
	}
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
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

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
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

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

func TestVerifyKimiLoginRequiresProvisionedModel(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "acp", "kimi")
	credential := filepath.Join(home, "credentials", "kimi-code.json")
	if err := os.MkdirAll(filepath.Dir(credential), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credential, []byte(`{"access_token":"token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{ModelCatalog: modelcatalog.NewService(nil), Root: root}
	auth := acp.AgentAuthConfig{Mode: acp.AuthModeJazProfile}
	if err := server.verifyACPAuthLogin(acp.AgentKimi, auth); err == nil || !strings.Contains(err.Error(), "did not finish configuring a model") {
		t.Fatalf("token-only login verification error = %v", err)
	}
	config := `default_model = "kimi"
[providers.kimi]
[models.kimi]
provider = "kimi"
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.verifyACPAuthLogin(acp.AgentKimi, auth); err != nil {
		t.Fatalf("configured login verification: %v", err)
	}
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
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

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
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/grok/auth/login", nil)
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

func TestACPAuthLoginAntigravityTailsLogAndRelaysCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the antigravity login PTY is unix-only")
	}
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "agy"), `#!/bin/sh
if [ "$1" = "models" ]; then echo "Fake Model"; exit 0; fi
log=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "--log-file" ]; then log="$arg"; fi
  prev="$arg"
done
printf 'I0707 12:00:00.000000 1 server.go:1] glog noise\n' >> "$log"
printf 'Authentication required. Please visit the URL to log in:\n' >> "$log"
printf '  https://accounts.google.com/o/oauth2/auth?code_challenge=test\n' >> "$log"
printf 'Or, paste the authorization code here and press Enter: \n' >> "$log"
read code
printf 'received=%s\n' "$code" >> "$log"
exit 0
`, "")
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/antigravity/auth/login", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", res.Code, res.Body.String())
	}
	var started acpAuthLoginResponse
	if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}

	// The sign-in URL only exists in agy's log; the tail must surface it.
	deadline := time.Now().Add(10 * time.Second)
	var current acpAuthLoginResponse
	for {
		get := httptest.NewRequest(http.MethodGet, "/v1/acp/auth-logins/"+started.ID, nil)
		getRes := httptest.NewRecorder()
		handler.ServeHTTP(getRes, get)
		if err := json.Unmarshal(getRes.Body.Bytes(), &current); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(current.AuthURL, "accounts.google.com") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("auth url never surfaced from the agy log: %#v", current)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if strings.Contains(current.Output, "glog noise") {
		t.Fatalf("glog lines leaked into login output: %q", current.Output)
	}

	in := httptest.NewRequest(http.MethodPost, "/v1/acp/auth-logins/"+started.ID+"/input", strings.NewReader(`{"input":"SIGN-CODE-7"}`))
	in.Header.Set("Content-Type", "application/json")
	inRes := httptest.NewRecorder()
	handler.ServeHTTP(inRes, in)
	if inRes.Code != http.StatusOK {
		t.Fatalf("input status = %d, body = %s", inRes.Code, inRes.Body.String())
	}

	done := waitForACPAuthLogin(t, handler, started.ID)
	if done.Status != "succeeded" {
		t.Fatalf("login status = %#v", done)
	}
	if !strings.Contains(done.Output, "received=SIGN-CODE-7") {
		t.Fatalf("login did not relay pasted code through the pty: %q", done.Output)
	}
}

func TestACPAuthLoginFailureLinePrefersErrorOverLaterPrompt(t *testing.T) {
	job := &acpAuthLoginJob{Output: "Authentication required.\nError: authentication failed or timed out\nOr, paste the authorization code here and press Enter: \n"}
	if got := job.failureLine(); got != "Error: authentication failed or timed out" {
		t.Fatalf("failureLine = %q", got)
	}
	job = &acpAuthLoginJob{Output: "something odd happened\n\n"}
	if got := job.failureLine(); got != "something odd happened" {
		t.Fatalf("failureLine fallback = %q", got)
	}
}
