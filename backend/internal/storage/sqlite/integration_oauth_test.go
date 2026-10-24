package sqlite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	oauthdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/integrationoauth"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
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

func TestOAuthSecretsMigrateAndRequireOriginalKey(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plain := `{"access_token":"legacy-access-sensitive","refresh_token":"legacy-refresh-sensitive","token_url":"https://example.com/token"}`
	if err := oauthdb.New(store.db).SaveToken(ctx, oauthdb.SaveTokenParams{ConnectionID: "account", TokenJson: plain, UpdatedAtMs: 123}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.LoadToken(ctx, "account")
	if err != nil || !ok || got.RefreshToken != "legacy-refresh-sensitive" {
		t.Fatalf("migrated credential unavailable: %v", err)
	}
	rows, err := oauthdb.New(store.db).ListTokens(ctx)
	if err != nil || len(rows) != 1 || rows[0].UpdatedAtMs != 123 || !strings.HasPrefix(rows[0].TokenJson, secretPrefix) {
		t.Fatalf("encrypted record or metadata incorrect: %v", err)
	}
	if err := oauthdb.New(store.db).SaveToken(ctx, oauthdb.SaveTokenParams{ConnectionID: "other-account", TokenJson: rows[0].TokenJson}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadToken(ctx, "other-account"); err == nil {
		t.Fatal("ciphertext from another account was accepted")
	}
	if err := store.DeleteToken(ctx, "other-account"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"jaz.sqlite", "jaz.sqlite-wal"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("legacy-refresh-sensitive")) {
			t.Fatalf("legacy plaintext remains in %s", name)
		}
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("credential database permissions: %v", err)
		}
	}
	if err := os.Remove(filepath.Join(root, "secrets.key")); err != nil {
		t.Fatal(err)
	}
	if reopened, err := New(root); err == nil {
		_ = reopened.Close()
		t.Fatal("lost encryption key silently replaced")
	}
}
