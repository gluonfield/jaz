package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func disconnectTestServer(store *sqlitestore.Store, root, agent string) *Server {
	return &Server{
		ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		Root:         root,
		AgentCatalog: acp.AgentCatalog{agent: acp.BuiltinAgents()[agent]},
	}
}

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
	handler := disconnectTestServer(store, root, acp.AgentClaude).Handler()

	// A Jaz-owned (profile) OAuth credential + a Jaz-managed API key.
	claudeDir := filepath.Join(root, "acp", "claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credPath := filepath.Join(claudeDir, ".claude.json")
	if err := os.WriteFile(credPath, []byte(`{"oauthAccount":{"accountUuid":"account-id"}}`), 0o600); err != nil {
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

	var got struct {
		ACPAuth map[string]acpAuthStatusResponse `json:"acp_auth"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ACPAuth[acp.AgentClaude].Authenticated {
		t.Fatalf("still authenticated after disconnect: %#v", got.ACPAuth)
	}
}

func TestDisconnectACPAuthOpenCodeKeepsConfigDir(t *testing.T) {
	// OpenCode's StoragePath is its config directory, not a credential file;
	// disconnect must not try to delete it (it holds jaz-instructions.md).
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"OPENROUTER_API_KEY", "OPENROUTER_APIKEY", "JAZ_ACP_OPENCODE_API_KEY"} {
		t.Setenv(key, "")
	}

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := disconnectTestServer(store, root, acp.AgentOpenCode).Handler()

	configDir := filepath.Join(root, "acp", "opencode")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	instructions := filepath.Join(configDir, "jaz-instructions.md")
	if err := os.WriteFile(instructions, []byte("instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{"JAZ_ACP_OPENCODE_API_KEY": "sk-test"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/opencode/auth/disconnect", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(instructions); err != nil {
		t.Fatalf("opencode config dir contents removed: %v", err)
	}
	if _, ok := runtimeenv.Lookup(runtimeenv.Path(root), "JAZ_ACP_OPENCODE_API_KEY"); ok {
		t.Fatalf("api key not removed from runtime env")
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
	handler := disconnectTestServer(store, root, acp.AgentClaude).Handler()

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

func TestDisconnectACPAuthStopsAutoFallbackToGlobalClaudeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{
		"CLAUDE_CONFIG_DIR", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY", "JAZ_ACP_CLAUDE_API_KEY",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "setup-token")

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
	if _, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{
		ACP: map[string]agentsettings.ACPAgentDefaults{
			acp.AgentClaude: {
				Enabled:         true,
				Model:           "default",
				ReasoningEffort: "medium",
				Auth:            acp.AgentAuthConfig{Mode: acp.AuthModeAuto},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/claude/auth/disconnect", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	disconnectTestServer(store, root, acp.AgentClaude).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var got struct {
		ACP     map[string]agentsettings.ACPAgentDefaults `json:"acp"`
		ACPAuth map[string]acpAuthStatusResponse          `json:"acp_auth"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	claudeAuth := got.ACPAuth[acp.AgentClaude]
	if claudeAuth.Authenticated || claudeAuth.AuthMode != acp.AuthModeJazProfile {
		t.Fatalf("disconnect status = %#v, want unauthenticated Jaz profile", claudeAuth)
	}
	if _, err := os.Stat(globalCred); err != nil {
		t.Fatalf("global ~/.claude config was deleted: %v", err)
	}
	claude := got.ACP[acp.AgentClaude]
	if claude.Enabled || claude.Auth.Mode != acp.AuthModeJazProfile {
		t.Fatalf("stored claude settings = %#v, want disabled Jaz profile", claude)
	}
}
