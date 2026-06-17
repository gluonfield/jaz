package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMigrationsAreIdempotent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.migrate(); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err := store.db.QueryRow(`SELECT version_id FROM goose_db_version WHERE version_id = 4 AND is_applied = 1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 4 {
		t.Fatalf("migration version = %d, want 4", version)
	}
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 4 {
		t.Fatalf("user_version = %d, want 4", version)
	}
	columns, err := tableColumns(store.db, "loops")
	if err != nil {
		t.Fatal(err)
	}
	if !columns["memory_path"] {
		t.Fatal("migration did not add loops.memory_path")
	}
}

func TestMigrationsAddLegacyThreadColumns(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "jaz.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().UnixMilli()
	if _, err := db.Exec(`CREATE TABLE threads (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT,
  parent_id TEXT,
  status TEXT NOT NULL DEFAULT 'idle',
  runtime TEXT NOT NULL DEFAULT 'acp',
  acp_agent TEXT,
  acp_session_id TEXT,
  model_provider TEXT,
  model TEXT,
  reasoning_effort TEXT,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO threads (id, slug, status, runtime, created_at_ms, updated_at_ms) VALUES (?, ?, ?, ?, ?, ?)`,
		"thread-1", "legacy", "idle", "acp", now, now); err != nil {
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

	columns, err := tableColumns(store.db, "threads")
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"archived", "error", "cwd", "project_path", "queued_messages", "source_type", "source_id", "pinned"} {
		if !columns[column] {
			t.Fatalf("legacy migration did not add threads.%s", column)
		}
	}
	if _, err := store.LoadSession("thread-1"); err != nil {
		t.Fatal(err)
	}
}

func TestUsageEventBackfillRowsAreMarkedSessionImport(t *testing.T) {
	root := t.TempDir()
	if err := makeLegacyDB(root); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "jaz.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := ensureLegacyThreadColumns(db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().UnixMilli()
	if _, err := db.Exec(`INSERT INTO threads (
  id, slug, status, runtime, input_tokens, cached_input_tokens, output_tokens, total_tokens, created_at_ms, updated_at_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"thread-usage", "imported-usage", "idle", "acp", 100, 20, 30, 150, now, now); err != nil {
		t.Fatal(err)
	}

	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}

	var source string
	if err := db.QueryRow(`SELECT source FROM usage_events WHERE thread_id = ?`, "thread-usage").Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "session_import" {
		t.Fatalf("backfill source = %q, want session_import", source)
	}
}

func TestSearchDocTablesDoNotDuplicateIndexedText(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	messageColumns, err := tableColumns(store.db, "message_search_docs")
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"content", "role"} {
		if messageColumns[column] {
			t.Fatalf("message_search_docs must not duplicate %s", column)
		}
	}

	threadColumns, err := tableColumns(store.db, "thread_search_docs")
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"title", "slug", "project_path"} {
		if threadColumns[column] {
			t.Fatalf("thread_search_docs must not duplicate %s", column)
		}
	}
}
