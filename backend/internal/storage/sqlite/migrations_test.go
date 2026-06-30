package sqlite

import (
	"database/sql"
	"strings"
	"testing"

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

func TestSessionEventsUsePrimaryKeyIndexOnly(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	indexes, err := indexNames(store.db, "session_events")
	if err != nil {
		t.Fatal(err)
	}
	if indexes["idx_session_events_thread_seq"] {
		t.Fatal("session_events must not keep duplicate idx_session_events_thread_seq")
	}
	if !indexes["sqlite_autoindex_session_events_1"] {
		t.Fatal("session_events primary key index is missing")
	}
	var plan string
	if err := store.db.QueryRow(`
EXPLAIN QUERY PLAN
SELECT thread_id, seq, type, content, acp, permission, plan, created_at_ms
FROM session_events
WHERE thread_id = ?
ORDER BY seq`, "thread").Scan(new(int), new(int), new(int), &plan); err != nil {
		t.Fatal(err)
	}
	if want := "sqlite_autoindex_session_events_1"; !strings.Contains(plan, want) {
		t.Fatalf("query plan = %q, want primary key index %q", plan, want)
	}
}

func TestUsageEventsUseCreatedAtIndexOnly(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	indexes, err := indexNames(store.db, "usage_events")
	if err != nil {
		t.Fatal(err)
	}
	if indexes["idx_usage_events_thread_created"] {
		t.Fatal("usage_events must not keep unused idx_usage_events_thread_created")
	}
	if !indexes["idx_usage_events_created_at"] {
		t.Fatal("usage_events created_at index is missing")
	}
	var plan string
	if err := store.db.QueryRow(`
EXPLAIN QUERY PLAN
SELECT thread_id, runtime, agent, model_provider, model, input_tokens, cached_input_tokens,
       cached_write_tokens, output_tokens, reasoning_output_tokens, total_tokens, source, created_at_ms
FROM usage_events
WHERE created_at_ms >= ?
ORDER BY created_at_ms`, 0).Scan(new(int), new(int), new(int), &plan); err != nil {
		t.Fatal(err)
	}
	if want := "idx_usage_events_created_at"; !strings.Contains(plan, want) {
		t.Fatalf("query plan = %q, want created_at index %q", plan, want)
	}
}

func TestIncludeCacheInInputMigrationRepairsUsageRows(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE threads (
			id TEXT PRIMARY KEY,
			input_tokens INTEGER NOT NULL,
			cached_input_tokens INTEGER NOT NULL,
			cached_write_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL
		)`,
		`CREATE TABLE usage_events (
			id INTEGER PRIMARY KEY,
			thread_id TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			cached_input_tokens INTEGER NOT NULL,
			cached_write_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL
		)`,
		`INSERT INTO threads VALUES ('disjoint', 38340, 47872, 0, 290, 86502)`,
		`INSERT INTO threads VALUES ('inclusive', 86212, 47872, 0, 290, 86502)`,
		`INSERT INTO threads VALUES ('no_total', 100, 50, 0, 15, 0)`,
		`INSERT INTO usage_events VALUES (1, 'disjoint', 38102, 4992, 0, 56, 43150)`,
		`INSERT INTO usage_events VALUES (2, 'disjoint', 238, 42880, 0, 234, 43352)`,
		`INSERT INTO usage_events VALUES (3, 'inclusive', 43094, 4992, 0, 56, 43150)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := sqliteMigrations.ReadFile("migrations/0026_include_cache_in_input.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.SplitN(string(raw), "-- +goose Down", 2)[0]
	if _, err := db.Exec(up); err != nil {
		t.Fatal(err)
	}

	wantThreads := map[string]int64{
		"disjoint":  86212,
		"inclusive": 86212,
		"no_total":  100,
	}
	for id, want := range wantThreads {
		var got int64
		if err := db.QueryRow(`SELECT input_tokens FROM threads WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("thread %s input = %d, want %d", id, got, want)
		}
	}
	wantEvents := map[int]int64{1: 43094, 2: 43118, 3: 43094}
	for id, want := range wantEvents {
		var got int64
		if err := db.QueryRow(`SELECT input_tokens FROM usage_events WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("usage event %d input = %d, want %d", id, got, want)
		}
	}
}

func TestNormalizeCodexNativeProviderMigrationRepairsLegacyRows(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE threads (
			id TEXT PRIMARY KEY,
			runtime TEXT NOT NULL,
			acp_agent TEXT NOT NULL,
			model_provider TEXT NOT NULL
		)`,
		`CREATE TABLE usage_events (
			id INTEGER PRIMARY KEY,
			runtime TEXT NOT NULL,
			agent TEXT NOT NULL,
			model_provider TEXT NOT NULL
		)`,
		`INSERT INTO threads VALUES ('legacy', 'acp', 'codex', 'codex')`,
		`INSERT INTO threads VALUES ('current', 'acp', 'codex', 'openai')`,
		`INSERT INTO threads VALUES ('claude', 'acp', 'claude', 'claude')`,
		`INSERT INTO usage_events VALUES (1, 'acp', 'codex', 'codex')`,
		`INSERT INTO usage_events VALUES (2, 'acp', 'codex', 'openai')`,
		`INSERT INTO usage_events VALUES (3, 'acp', 'claude', 'claude')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := sqliteMigrations.ReadFile("migrations/0036_normalize_codex_native_provider.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.SplitN(string(raw), "-- +goose Down", 2)[0]
	if _, err := db.Exec(up); err != nil {
		t.Fatal(err)
	}
	for table, query := range map[string]string{
		"threads":      `SELECT COUNT(*) FROM threads WHERE runtime = 'acp' AND acp_agent = 'codex' AND model_provider = 'codex'`,
		"usage_events": `SELECT COUNT(*) FROM usage_events WHERE runtime = 'acp' AND agent = 'codex' AND model_provider = 'codex'`,
	} {
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != 0 {
			t.Fatalf("%s legacy rows = %d, want 0", table, got)
		}
	}
	var claudeProvider string
	if err := db.QueryRow(`SELECT model_provider FROM threads WHERE id = 'claude'`).Scan(&claudeProvider); err != nil {
		t.Fatal(err)
	}
	if claudeProvider != "claude" {
		t.Fatalf("claude provider = %q, want unchanged", claudeProvider)
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

func indexNames(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_index_list(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}
