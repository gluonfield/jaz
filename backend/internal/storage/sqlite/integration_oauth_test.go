package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	_ "modernc.org/sqlite"
)

func TestIntegrationOAuthTokenRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	want := integrationoauth.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       time.Date(2026, 6, 12, 15, 0, 0, 0, time.UTC),
		ClientID:     "client",
		TokenURL:     "https://auth.example.com/token",
		Scopes:       []string{"read"},
	}
	if err := store.SaveToken(context.Background(), "gmail:primary", want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.LoadToken(context.Background(), "gmail:primary")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("token not found")
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken || got.ClientID != want.ClientID || got.TokenURL != want.TokenURL || !got.Expiry.Equal(want.Expiry) {
		t.Fatalf("token = %#v, want %#v", got, want)
	}
}

func TestIntegrationOAuthMigrationCopiesLegacyMCPTokens(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "jaz.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	token := integrationoauth.Token{
		AccessToken:  "legacy-access",
		RefreshToken: "legacy-refresh",
		TokenType:    "Bearer",
		ClientID:     "legacy-client",
		TokenURL:     "https://auth.example.com/token",
	}
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE mcp_oauth_tokens (
  server_id TEXT PRIMARY KEY,
  token_json TEXT NOT NULL,
  updated_at_ms INTEGER NOT NULL
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mcp_oauth_tokens (server_id, token_json, updated_at_ms) VALUES (?, ?, ?)`, "mcp_n8n", string(data), time.Now().UTC().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got, ok, err := store.LoadToken(context.Background(), mcpconfig.OAuthConnectionID("mcp_n8n"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("migrated token not found")
	}
	if got.AccessToken != token.AccessToken || got.RefreshToken != token.RefreshToken || got.ClientID != token.ClientID {
		t.Fatalf("token = %#v, want %#v", got, token)
	}
}
