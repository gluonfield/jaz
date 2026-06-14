package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/runtimeenv"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDisconnectACPAuthRemovesJazOwnedCredentialAndKey(t *testing.T) {
	// Keep the dev's real ~/.claude and provider env out of the probe.
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{
		"CLAUDE_CONFIG_DIR", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY", "JAZ_ACP_CLAUDE_API_KEY",
	} {
		t.Setenv(key, "")
	}

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store, Root: root}).Handler()

	// A Jaz-owned (profile) OAuth credential + a Jaz-managed API key.
	claudeDir := filepath.Join(root, "acp", "claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credPath := filepath.Join(claudeDir, ".claude.json")
	if err := os.WriteFile(credPath, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{"JAZ_ACP_CLAUDE_API_KEY": "sk-test"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/claude/auth/disconnect", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	if _, err := os.Stat(credPath); !os.IsNotExist(err) {
		t.Fatalf("jaz-owned credential not deleted: %v", err)
	}
	if _, ok := runtimeenv.Lookup(runtimeenv.Path(root), "JAZ_ACP_CLAUDE_API_KEY"); ok {
		t.Fatalf("api key not removed from runtime env")
	}

	var got acpAuthStatusResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Authenticated {
		t.Fatalf("still authenticated after disconnect: %#v", got)
	}
}

func TestDisconnectACPAuthKeepsGlobalConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{
		"CLAUDE_CONFIG_DIR", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY", "JAZ_ACP_CLAUDE_API_KEY",
	} {
		t.Setenv(key, "")
	}

	// The user's global ~/.claude/.claude.json must never be deleted.
	globalDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(globalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	globalCred := filepath.Join(globalDir, ".claude.json")
	if err := os.WriteFile(globalCred, []byte(`{"config":"keep me"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{Store: store, Root: root}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/claude/auth/disconnect", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(globalCred); err != nil {
		t.Fatalf("global ~/.claude config was deleted: %v", err)
	}
}
