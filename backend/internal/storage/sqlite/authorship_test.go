package sqlite

import (
	"io/fs"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestAuthorshipUpgradePreservesHistoryAndDefaultsToUnknown(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "legacy-authorship"})
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.DownTo(t.Context(), 73); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO messages (thread_id, seq, role, content, blocks, created_at_ms, from_agent) VALUES (?, 1, 'user', 'human', '[]', 1000, 0), (?, 2, 'user', 'agent', '[]', 2000, 1)`, session.ID, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO messages (thread_id, seq, role, content, blocks, created_at_ms) VALUES (?, 3, 'user', 'unmarked', '[]', 3000)`, session.ID); err != nil {
		t.Fatal(err)
	}
	last, err := store.LastUserMessageAt()
	if err != nil || !last.Equal(time.UnixMilli(1000)) {
		t.Fatalf("upgraded last human message = %v, %v", last, err)
	}
}
