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

func TestACPAuthLoginClaudePreservesNamedSDKProfileHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	invocation := acp.AgentLoginInvocationFor(acp.AgentClaude, root, acp.AgentAuthConfig{Mode: acp.AuthModeJazProfile}, "", map[string]string{"ANTHROPIC_PROFILE": "selected"})
	env := strings.Join(acpAuthLoginEnv(invocation), "\n")
	for _, want := range []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=" + filepath.Join(root, "acp", "claude"), "ANTHROPIC_PROFILE=selected"} {
		if !strings.Contains(env, want) {
			t.Fatalf("login environment missing %q: %s", want, env)
		}
	}
	if strings.Contains(env, "ANTHROPIC_CONFIG_DIR=") {
		t.Fatalf("named SDK profile acquired a different config directory: %s", env)
	}
}

func TestACPAuthLoginRunsCodexWithoutHome(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "codex"), `#!/bin/sh
printf 'home=%s\n' "$HOME"
printf 'codex_home=%s\n' "$CODEX_HOME"
printf '{"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}}' > "$CODEX_HOME/auth.json"
`, `@echo off
echo home=%HOME%
echo codex_home=%CODEX_HOME%
echo {"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}} > "%CODEX_HOME%\auth.json"
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
printf '{"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}}' > "$CODEX_HOME/auth.json"
printf ok
`, `@echo off
if not exist "%CODEX_HOME%\" (
  echo CODEX_HOME points to "%CODEX_HOME%", but that path does not exist 1>&2
  exit /b 1
)
echo {"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}} > "%CODEX_HOME%\auth.json"
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
printf '{"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}}' > "$CODEX_HOME/auth.json"
`, `@echo off
echo visit https://auth.example/login
set /p code=
echo received=%code%
echo {"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}} > "%CODEX_HOME%\auth.json"
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
	writeCodexOAuth(t, existing)
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "codex"), `#!/bin/sh
printf 'codex_home=%s\n' "$CODEX_HOME"
printf '{"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}}' > "$CODEX_HOME/auth.json"
`, `@echo off
echo codex_home=%CODEX_HOME%
echo {"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}} > "%CODEX_HOME%\auth.json"
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

func writeCodexOAuth(t *testing.T, home string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"access","refresh_token":"refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
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

func TestACPAuthLoginAntigravityRelaysTerminalLoginCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the antigravity login PTY is unix-only")
	}
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "antigravity"), `#!/bin/sh
if [ "$1" = "models" ]; then echo "Fake Model"; exit 0; fi
if [ "$1" != "acp" ] || [ "$2" != "--login" ]; then exit 2; fi
printf 'Authentication required. Please visit the URL to log in:\n'
printf '  https://accounts.google.com/o/oauth2/auth?code_challenge=test\n'
printf 'Or, paste the authorization code here and press Enter: \n'
read code
printf 'received=%s\n' "$code"
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

	start := func() acpAuthLoginResponse {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/acp/agents/antigravity/auth/login", nil))
		if res.Code != http.StatusOK {
			t.Fatalf("start status = %d, body = %s", res.Code, res.Body.String())
		}
		var started acpAuthLoginResponse
		if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
		return started
	}
	abandoned := start()
	started := start()
	if got := waitForACPAuthLogin(t, handler, abandoned.ID); got.Status != "failed" {
		t.Fatalf("retry did not replace the pending sign-in: %#v", got)
	}

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
			t.Fatalf("auth url never surfaced from native terminal login: %#v", current)
		}
		time.Sleep(100 * time.Millisecond)
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

func TestACPAuthLoginAntigravityGoogleCloudSignsInOverACP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell test harness is Unix-only")
	}
	home := t.TempDir()
	bin := t.TempDir()
	testexec.Write(t, filepath.Join(bin, "antigravity"), `#!/bin/sh
if [ "$1" = "models" ]; then
    [ -f "$HOME/cloud-sign-in" ] || exit 1
    echo "Fake Model"
    exit 0
fi
[ "$1" = "acp" ] || exit 2
IFS= read -r init
printf '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}\n'
IFS= read -r auth
case "$auth" in *'"method":"authenticate"'*'"methodId":"oauth-business"'*) ;; *) exit 3 ;; esac
cp "$HOME/.gemini/antigravity-cli/settings.json" "$HOME/cloud-sign-in"
printf '{"jsonrpc":"2.0","id":2,"result":{}}\n'
`, "")
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	settingsDir := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{"trustedWorkspaces":["/work"],"gcp":{"project":"old"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, Root: root}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/acp/agents/antigravity/auth/login",
		strings.NewReader(`{"google_cloud":{"project":" credits-project ","location":"global"}}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", res.Code, res.Body.String())
	}
	var started acpAuthLoginResponse
	if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if !started.OpensBrowser {
		t.Fatalf("Google Cloud sign-in did not report that the agent opens the browser: %#v", started)
	}
	if done := waitForACPAuthLogin(t, handler, started.ID); done.Status != "succeeded" {
		t.Fatalf("login status = %#v", done)
	}
	raw, err := os.ReadFile(filepath.Join(home, "cloud-sign-in"))
	if err != nil {
		t.Fatal(err)
	}
	var seen struct {
		TrustedWorkspaces []string `json:"trustedWorkspaces"`
		GCP               struct{ Project, Location string }
	}
	if err := json.Unmarshal(raw, &seen); err != nil {
		t.Fatal(err)
	}
	if seen.GCP.Project != "credits-project" || seen.GCP.Location != "global" || len(seen.TrustedWorkspaces) != 1 {
		t.Fatalf("settings Antigravity signed in with = %s", raw)
	}
}

func TestACPAuthLoginAntigravityPayAsYouGoAnswersTheSignInWizard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the antigravity sign-in wizard runs on a unix pty")
	}
	home := t.TempDir()
	bin := t.TempDir()
	// Like agy, the fake repaints its whole screen only when the terminal is
	// resized, starts each menu with the cursor on the first item, draws the
	// sign-in page as a hyperlink, and opens its project and location fields
	// already holding a value.
	testexec.Write(t, filepath.Join(bin, "antigravity"), `#!/bin/bash
if [ "$1" = "models" ]; then
    [ -f "$HOME/pay-as-you-go" ] || exit 1
    echo "Fake Model"
    exit 0
fi
[ $# = 0 ] || exit 2
stty raw -echo
show() {
    printf '\033[2J\033[H%s' "$1" > "$HOME/screen"
    cat "$HOME/screen"
}
(
    size=$(stty size < /dev/tty)
    while sleep 0.1; do
        now=$(stty size < /dev/tty) || exit
        [ "$now" = "$size" ] || cat "$HOME/screen"
        size=$now
    done
) &
# bash's read turns Enter into its line end, so Enter is the empty key.
key() { IFS= read -rsn1 key || exit 4; }
pick() {
    cursor=1
    while :; do
        key
        case "$key" in
        $'\033') IFS= read -rsn2 arrow
            [ "$arrow" = "[B" ] && cursor=$((cursor + 1))
            [ "$arrow" = "[A" ] && cursor=$((cursor - 1)) ;;
        '' | $'\r') [ "$cursor" = "$1" ] || exit 3
            return ;;
        esac
    done
}
line() {
    value=$1
    while :; do
        key
        case "$key" in
        '' | $'\r') return ;;
        $'\025') value= ;;
        *) value="$value$key" ;;
        esac
    done
}
show $' Select login method:\r\n > 1. Google OAuth\r\n   2. Use a Google Cloud project'
pick 2
show $' Select Google Cloud sign-in method:\r\n > 1. Continue with Google Cloud\r\n   2. Use advanced SSO config'
pick 1
show $' \033]8;id=1;https://accounts.google.com/o/oauth2/auth?state=fake\a Click here to authenticate\033]8;;\a\r\n paste the authorization code below:'
line
code=$value
show $' Select billing model:\r\n > 1. Gemini subscription\r\n   2. Pay As You Go'
pick 2
show ' Provide Google Cloud project:'
line stale
project=$value
show ' Provide location:'
line stale
printf '%s %s %s' "$code" "$project" "$value" > "$HOME/pay-as-you-go"
show ' Signed in'
sleep 60
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

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/acp/agents/antigravity/auth/login",
		strings.NewReader(`{"google_cloud":{"project":"credits-project","location":"global","pay_as_you_go":true}}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", res.Code, res.Body.String())
	}
	var started acpAuthLoginResponse
	if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); started.AuthURL == ""; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("sign-in page never surfaced: %#v", started)
		}
		get := httptest.NewRecorder()
		handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/v1/acp/auth-logins/"+started.ID, nil))
		if err := json.Unmarshal(get.Body.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
	}
	if started.AuthURL != "https://accounts.google.com/o/oauth2/auth?state=fake" {
		t.Fatalf("sign-in page = %q", started.AuthURL)
	}
	in := httptest.NewRecorder()
	handler.ServeHTTP(in, httptest.NewRequest(http.MethodPost, "/v1/acp/auth-logins/"+started.ID+"/input", strings.NewReader(`{"input":"CODE-7"}`)))
	if in.Code != http.StatusOK {
		t.Fatalf("input status = %d, body = %s", in.Code, in.Body.String())
	}
	if done := waitForACPAuthLogin(t, handler, started.ID); done.Status != "succeeded" {
		t.Fatalf("login status = %#v", done)
	}
	answered, err := os.ReadFile(filepath.Join(home, "pay-as-you-go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(answered) != "CODE-7 credits-project global" {
		t.Fatalf("wizard was answered with %q", answered)
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
