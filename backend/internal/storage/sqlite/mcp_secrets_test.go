package sqlite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/mcpdb"
)

func TestManualMCPSecretsStayEncryptedThroughSaveAndReopen(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	input := mcpconfig.ServerInput{Name: "Private tools", URL: "https://example.invalid/mcp", BearerToken: "sensitive-manual-bearer", Headers: []mcpconfig.Header{{Name: "X-Api-Key", Value: "sensitive-manual-header"}}}
	server, err := store.CreateMCPServer(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Name = "Renamed tools"
	if _, err := store.UpdateMCPServer(server.ID, input); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadMCPServer(server.ID)
	if err != nil || got.BearerToken != input.BearerToken || len(got.Headers) != 1 || got.Headers[0].Value != input.Headers[0].Value {
		t.Fatalf("MCP runtime credentials changed: %v", err)
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
		if bytes.Contains(data, []byte(input.BearerToken)) || bytes.Contains(data, []byte(input.Headers[0].Value)) {
			t.Fatalf("plaintext MCP credential retained in %s", name)
		}
	}
	if err := os.Remove(filepath.Join(root, "secrets.key")); err != nil {
		t.Fatal(err)
	}
	if reopened, err := New(root); err == nil {
		_ = reopened.Close()
		t.Fatal("manual MCP encryption silently replaced its missing key")
	}
}

func TestLegacyManualMCPSecretsMigrateAtomically(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx := context.Background()
	q := mcpdb.New(store.db)
	for _, id := range []string{"first", "second"} {
		if err := q.CreateMCPServer(ctx, mcpdb.CreateMCPServerParams{ID: id, Name: id, Url: "https://example.invalid/mcp", Transport: "streamable_http", BearerToken: nullDBString("legacy-bearer"), HeadersJson: `[{"name":"X-Key","value":"legacy-header"}]`, EnvHeadersJson: "[]", OauthJson: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.db.Exec(`CREATE TRIGGER fail_mcp_migration BEFORE UPDATE ON mcp_servers WHEN NEW.id = 'second' BEGIN SELECT RAISE(ABORT, 'migration failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.protectSecrets(); err == nil {
		t.Fatal("failed migration was ignored")
	}
	first, err := q.GetMCPServer(ctx, "first")
	if err != nil || first.BearerToken.String != "legacy-bearer" {
		t.Fatalf("failed migration changed an earlier row: %v", err)
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_mcp_migration"); err != nil {
		t.Fatal(err)
	}
	if err := store.protectSecrets(); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadMCPServer("first")
	if err != nil || got.BearerToken != "legacy-bearer" || len(got.Headers) != 1 || got.Headers[0].Value != "legacy-header" {
		t.Fatalf("legacy MCP credentials changed: %v", err)
	}
}
