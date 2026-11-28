package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	_ "modernc.org/sqlite"
)

func TestWorkspaceMigrationPreservesExistingBotAndConversation(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "existing-bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	record := storage.BotRecord{ThreadID: bot.ID, Kind: "bot", Prompt: "Maintain the project", Shape: "cloud", Color: "teal"}
	if err := store.SaveBot(record); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(bot.ID, storage.Message{Role: "user", Content: "Keep this conversation", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(t.Context(), 85); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadBot(bot.ID)
	if err != nil || loaded.Prompt != record.Prompt || loaded.Shape != record.Shape || loaded.Color != record.Color || len(loaded.Workspaces) != 0 {
		t.Fatalf("migrated bot = %+v, %v", loaded, err)
	}
	var text string
	if err := store.db.QueryRow("SELECT content FROM messages WHERE thread_id = ?", bot.ID).Scan(&text); err != nil || text != "Keep this conversation" {
		t.Fatalf("migrated message = %q, %v", text, err)
	}
	loaded.Workspaces = []string{t.TempDir()}
	if err := store.SaveBot(loaded); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.LoadBot(bot.ID)
	if err != nil || len(reloaded.Workspaces) != 1 || reloaded.Workspaces[0] != loaded.Workspaces[0] || reloaded.Prompt != record.Prompt {
		t.Fatalf("reopened bot = %+v, %v", reloaded, err)
	}
}

func TestBotTaskQuestionMigrationReleasesOnlyAnsweredQuestions(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	tasks := map[string]storage.Commitment{}
	for _, slug := range []string{"answered", "unanswered"} {
		bot, err := store.CreateSession(storage.CreateSession{Slug: slug, SourceType: storage.SourceBot})
		if err != nil {
			t.Fatal(err)
		}
		for range 3 {
			if err := store.AppendMessageRecords(bot.ID, storage.Message{Role: "user", Content: "Compare suppliers", FromUser: true, CreatedAt: time.UnixMilli(100)}); err != nil {
				t.Fatal(err)
			}
		}
		task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Compare suppliers"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.AskCommitment(task, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, At: time.UnixMilli(1000),
			RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Which supplier?"}}); err != nil {
			t.Fatal(err)
		}
		tasks[slug] = task
	}
	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(t.Context(), 76); err != nil {
		t.Fatal(err)
	}
	answered := tasks["answered"]
	if _, err := store.db.Exec("DELETE FROM messages WHERE thread_id = ?", answered.BotID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("INSERT INTO messages (thread_id, seq, role, content, blocks, created_at_ms, from_user) VALUES (?, 1, 'user', 'Supplier A', '[]', 2000, 1)", answered.BotID); err != nil {
		t.Fatal(err)
	}
	var before string
	err = store.db.QueryRow("SELECT status FROM bot_commitments WHERE id = ?", answered.ID).Scan(&before)
	if err != nil || before != storage.CommitmentWaitingForUser {
		t.Fatalf("legacy stuck wait was not reproduced: %+v, %v", before, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	for slug, status := range map[string]string{"answered": storage.CommitmentPending, "unanswered": storage.CommitmentWaitingForUser} {
		task, err := store.LoadCommitment(tasks[slug].ID)
		if err != nil || task.Status != status {
			t.Fatalf("migrated %s question = %+v, %v; want %s", slug, task, err, status)
		}
	}
}

func TestBotTaskMigrationPreservesWorkAndReconcilesUnverifiedLegacyWaits(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := store.CreateSession(storage.CreateSession{Slug: "worker", SourceType: storage.SourceBotWorker, SourceID: bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(t.Context(), 75); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id      int64
		status  string
		waiting string
	}{
		{1, "open", "requester"}, {2, "open", "outside"}, {3, "open", ""},
		{4, "done", ""}, {5, "dropped", ""},
	} {
		if _, err := store.db.Exec("INSERT INTO bot_commitments (id, bot_id, request, note, status, waiting, opened_at_ms, closed_at_ms) VALUES (?, ?, 'original request', 'original next step', ?, ?, 123000, 124000)", row.id, bot.ID, row.status, row.waiting); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec("INSERT INTO bot_commitment_threads (thread_id, commitment_id) VALUES (?, 3)", worker.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	for id, status := range map[int64]string{1: storage.CommitmentPending, 2: storage.CommitmentPending, 3: storage.CommitmentInProgress, 4: storage.CommitmentDone, 5: storage.CommitmentDropped} {
		task, err := store.LoadCommitment(id)
		if err != nil || task.Status != status || task.Request != "original request" || task.Note != "original next step" || !task.OpenedAt.Equal(time.UnixMilli(123000)) || task.QuestionSeq != 0 {
			t.Fatalf("migrated task = %+v, %v; want status %s with its original data", task, err, status)
		}
	}
}

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
	columns, err = tableColumns(store.db, "threads")
	if err != nil {
		t.Fatal(err)
	}
	if !columns["event_compaction_version"] || !columns["event_revision"] || !columns["transcript_revision"] {
		t.Fatal("migration did not add session event compaction state")
	}
	indexes, err := indexNames(store.db, "threads")
	if err != nil {
		t.Fatal(err)
	}
	if !indexes["idx_threads_event_compaction_pending"] {
		t.Fatal("migration did not add pending event compaction index")
	}
	columns, err = tableColumns(store.db, "session_events")
	if err != nil {
		t.Fatal(err)
	}
	if !columns["projection_key"] || !columns["projection_op"] {
		t.Fatal("migration did not add session event projection metadata")
	}
}

func TestSessionEventsUseRequiredIndexes(t *testing.T) {
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
	if !indexes["idx_session_events_coalesce"] {
		t.Fatal("session_events coalescing index is missing")
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

func TestSessionOverviewUsesRequiredIndexes(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tests := []struct {
		name  string
		query string
		arg   string
		index string
	}{
		{
			name: "threads",
			query: `EXPLAIN QUERY PLAN
SELECT id, slug, title, status, acp_agent, model, reasoning_effort, archived, updated_at_ms
FROM threads
WHERE parent_id = ? AND runtime = 'acp' AND (COALESCE(source_type, '') = '' OR (source_type = 'bot_worker' AND source_id <> parent_id))
ORDER BY last_attention_at_ms DESC, id`,
			arg: "parent", index: "idx_threads_overview_children",
		},
		{
			name: "subagents",
			query: `EXPLAIN QUERY PLAN
SELECT thread_id, seq, projection_key, projection_op, type, content, acp, plan, permission, created_at_ms
FROM session_events
WHERE thread_id = ? AND type = 'provider_subagent'
ORDER BY seq`,
			arg: "parent", index: "idx_session_events_provider_subagents",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, err := store.db.Query(test.query, test.arg)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plans []string
			for rows.Next() {
				var plan string
				if err := rows.Scan(new(int), new(int), new(int), &plan); err != nil {
					t.Fatal(err)
				}
				plans = append(plans, plan)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			plan := strings.Join(plans, "\n")
			if !strings.Contains(plan, test.index) || strings.Contains(plan, "USE TEMP B-TREE") {
				t.Fatalf("query plan = %q, want index %q without a temporary sort", plan, test.index)
			}
		})
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

func TestRemoveDeployinkMCPServersMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE mcp_servers (id TEXT PRIMARY KEY, url TEXT NOT NULL)`,
		`CREATE TABLE integration_oauth_tokens (connection_id TEXT PRIMARY KEY, token_json TEXT NOT NULL, updated_at_ms INTEGER NOT NULL)`,
		`INSERT INTO mcp_servers VALUES ('deployink', 'https://mcp.deployink.com')`,
		`INSERT INTO mcp_servers VALUES ('deployink_legacy', 'https://mcp.ml.ink/mcp')`,
		`INSERT INTO mcp_servers VALUES ('docs', 'https://mcp.example.com')`,
		`INSERT INTO integration_oauth_tokens VALUES ('mcp:deployink', '{}', 1)`,
		`INSERT INTO integration_oauth_tokens VALUES ('mcp:deployink_legacy', '{}', 1)`,
		`INSERT INTO integration_oauth_tokens VALUES ('mcp:docs', '{}', 1)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := sqliteMigrations.ReadFile("migrations/0039_remove_deployink_mcp_servers.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.SplitN(string(raw), "-- +goose Down", 2)[0]
	if _, err := db.Exec(up); err != nil {
		t.Fatal(err)
	}

	for table, query := range map[string]string{
		"mcp_servers":              `SELECT COUNT(*) FROM mcp_servers WHERE id LIKE 'deployink%'`,
		"integration_oauth_tokens": `SELECT COUNT(*) FROM integration_oauth_tokens WHERE connection_id LIKE 'mcp:deployink%'`,
	} {
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != 0 {
			t.Fatalf("%s legacy deployink rows = %d, want 0", table, got)
		}
	}
	for table, query := range map[string]string{
		"mcp_servers":              `SELECT COUNT(*) FROM mcp_servers WHERE id = 'docs'`,
		"integration_oauth_tokens": `SELECT COUNT(*) FROM integration_oauth_tokens WHERE connection_id = 'mcp:docs'`,
	} {
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != 1 {
			t.Fatalf("%s preserved rows = %d, want 1", table, got)
		}
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

func TestClearUnappliedGrokModelMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE settings (namespace TEXT NOT NULL, key TEXT NOT NULL, value_json TEXT NOT NULL)`,
		`CREATE TABLE loops (id TEXT PRIMARY KEY, acp_agent TEXT, model TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO settings VALUES ('agents', 'defaults', '{"acp":{"grok":{"enabled":true,"model":"grok-4.6","reasoning_effort":"xhigh"},"codex":{"model":"gpt-6-sol"}}}')`,
		`INSERT INTO settings VALUES ('memory', 'settings', '{"enabled":true,"agent":"grok","model":"grok-4.6"}')`,
		`INSERT INTO loops VALUES ('grok-loop', 'grok', 'grok-4.6'), ('codex-loop', 'codex', 'gpt-6-sol')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := sqliteMigrations.ReadFile("migrations/0051_clear_unapplied_grok_model.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(strings.SplitN(string(raw), "-- +goose Down", 2)[0]); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct{ query, want string }{
		{`SELECT value_json FROM settings WHERE namespace = 'agents'`, `{"acp":{"grok":{"enabled":true,"reasoning_effort":"xhigh"},"codex":{"model":"gpt-6-sol"}}}`},
		{`SELECT value_json FROM settings WHERE namespace = 'memory'`, `{"enabled":true,"agent":"grok"}`},
		{`SELECT group_concat(id || '=' || model, ',') FROM loops`, `grok-loop=,codex-loop=gpt-6-sol`},
	} {
		var got string
		if err := db.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != check.want {
			t.Fatalf("%s = %s, want %s", check.query, got, check.want)
		}
	}
}

func TestWorkspaceEntitiesMigrationUpDown(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Populate data across local, cloudA, and cloudB including duplicate
	// plugin_sources/plugin_state IDs and two main=1 bots across workspaces.
	localBot, err := store.CreateSession(storage.CreateSession{Slug: "local-prime", Title: "Prime", SourceType: storage.SourceBot, WorkspaceID: storage.LocalWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	cloudABot, err := store.CreateSession(storage.CreateSession{Slug: "cloud-a-prime", Title: "Prime", SourceType: storage.SourceBot, WorkspaceID: "cloudA"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBot(storage.BotRecord{ThreadID: localBot.ID, WorkspaceID: storage.LocalWorkspaceID, Kind: "bot", Shape: "circle", Color: "blue", Main: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBot(storage.BotRecord{ThreadID: cloudABot.ID, WorkspaceID: "cloudA", Kind: "bot", Shape: "hex", Color: "green", Main: true}); err != nil {
		t.Fatal(err)
	}

	for _, board := range []storage.App{
		{ID: "board-local-1", WorkspaceID: storage.LocalWorkspaceID, Title: "Local Board", Kind: storage.AppKindBoard},
		{ID: "board-cloud-a", WorkspaceID: "cloudA", Title: "Cloud A Board", Kind: storage.AppKindBoard},
	} {
		if _, err := store.CreateApp(ctx, board); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveLoop(loops.Loop{ID: "loop-local-1", WorkspaceID: storage.LocalWorkspaceID, Name: "Local Loop", Prompt: "p", Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}, Status: loops.StatusActive, Runtime: loops.RuntimeACP, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLoop(loops.Loop{ID: "loop-cloud-a", WorkspaceID: "cloudA", Name: "Cloud A Loop", Prompt: "p", Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}, Status: loops.StatusActive, Runtime: loops.RuntimeACP, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, loopApp := range []storage.App{
		{ID: "loop-app-local-1", WorkspaceID: storage.LocalWorkspaceID, Title: "Local W", Kind: storage.AppKindArtifact, LoopID: "loop-local-1"},
		{ID: "loop-app-cloud-a", WorkspaceID: "cloudA", Title: "Cloud A W", Kind: storage.AppKindArtifact, LoopID: "loop-cloud-a"},
	} {
		if _, err := store.CreateApp(ctx, loopApp); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PlaceBoardTiles(ctx, "board-local-1", []storage.TilePlacement{{AppID: "loop-app-local-1", X: 2, Y: 1, W: 3, H: 2, PlacedBy: storage.TilePlacedByUser}}); err != nil {
		t.Fatal(err)
	}
	localMCP, err := store.CreateMCPServer(mcpconfig.ServerInput{WorkspaceID: storage.LocalWorkspaceID, Name: "local-srv", URL: "https://local.example.com/mcp", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateMCPServer(mcpconfig.ServerInput{WorkspaceID: "cloudA", Name: "cloud-srv", URL: "https://cloud.example.com/mcp", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, ws := range []string{storage.LocalWorkspaceID, "cloudA", "cloudB"} {
		if err := store.SavePluginSource(ctx, storage.PluginSourceRecord{
			WorkspaceID: ws, ID: "shared/source", Repository: "shared/source", URL: "https://example.com/shared.git", CatalogJSON: json.RawMessage(`[]`), UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save plugin source in %s: %v", ws, err)
		}
		if err := store.SavePluginState(ctx, storage.PluginStateRecord{
			WorkspaceID: ws, PluginID: "shared-plugin", Enabled: ws == storage.LocalWorkspaceID, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save plugin state in %s: %v", ws, err)
		}
	}

	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 95); err != nil {
		t.Fatalf("downgrade 0096 failed: %v", err)
	}

	// Verify downgraded tables dropped workspace_id and kept local plugin rows.
	for _, table := range []string{"bots", "boards", "widgets", "loops", "mcp_servers", "plugin_sources", "plugin_state"} {
		cols, err := tableColumns(store.db, table)
		if err != nil {
			t.Fatal(err)
		}
		if cols["workspace_id"] {
			t.Fatalf("%s still has workspace_id after downgrade to 95", table)
		}
	}
	var srcCount, stateCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM plugin_sources WHERE id = 'shared/source'`).Scan(&srcCount); err != nil || srcCount != 1 {
		t.Fatalf("downgraded plugin_sources count = %d, err = %v; want 1", srcCount, err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM plugin_state WHERE plugin_id = 'shared-plugin' AND enabled = 1`).Scan(&stateCount); err != nil || stateCount != 1 {
		t.Fatalf("downgraded plugin_state count = %d, err = %v; want 1", stateCount, err)
	}

	// Re-open to re-apply 0096 and verify columns, indexes, and preserved local data.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatalf("re-upgrade 0096 failed: %v", err)
	}
	for table, wantIdx := range map[string]string{
		"bots":        "idx_bots_workspace",
		"loops":       "idx_loops_workspace",
		"mcp_servers": "idx_mcp_servers_workspace",
	} {
		cols, err := tableColumns(store.db, table)
		if err != nil {
			t.Fatal(err)
		}
		if !cols["workspace_id"] {
			t.Fatalf("%s missing workspace_id after re-upgrade", table)
		}
		indexes, err := indexNames(store.db, table)
		if err != nil {
			t.Fatal(err)
		}
		if !indexes[wantIdx] {
			t.Fatalf("%s missing index %s after re-upgrade", table, wantIdx)
		}
	}
	reloadedBot, err := store.LoadBot(localBot.ID)
	if err != nil || reloadedBot.WorkspaceID != storage.LocalWorkspaceID || !reloadedBot.Main {
		t.Fatalf("reloaded local bot = %+v, %v", reloadedBot, err)
	}
	// The board and its loop tile came back as apps, layout intact.
	var boardID string
	if err := store.db.QueryRow(`SELECT id FROM apps WHERE kind = 'board' AND title = 'Local Board' AND workspace_id = 'local'`).Scan(&boardID); err != nil {
		t.Fatalf("reloaded local board: %v", err)
	}
	tiles, err := store.ListBoardTiles(ctx, boardID)
	if err != nil || len(tiles) != 1 || tiles[0].Loop == nil || tiles[0].Loop.ID != "loop-local-1" || tiles[0].App.Title != "Local W" || tiles[0].X != 2 || tiles[0].Y != 1 || tiles[0].W != 3 || tiles[0].H != 2 || tiles[0].PlacedBy != storage.TilePlacedByUser {
		t.Fatalf("reloaded local board tiles = %+v, %v", tiles, err)
	}
	reloadedMCP, err := store.LoadMCPServer(localMCP.ID)
	if err != nil || reloadedMCP.WorkspaceID != storage.LocalWorkspaceID {
		t.Fatalf("reloaded local mcp = %+v, %v", reloadedMCP, err)
	}
	reloadedState, ok, err := store.GetPluginState(ctx, storage.LocalWorkspaceID, "shared-plugin")
	if err != nil || !ok || !reloadedState.Enabled {
		t.Fatalf("reloaded local plugin state = %+v, ok=%v, err=%v", reloadedState, ok, err)
	}
}

func TestBoardsAsAppsMigrationKeepsBoardsTilesAndLoopApps(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, loop := range []loops.Loop{
		{ID: "loop-html", Name: "Inbox", Status: loops.StatusActive},
		{ID: "loop-app", Name: "Revenue", Status: loops.StatusActive},
		{ID: "loop-gone", Name: "Gone", Status: loops.StatusDeleted},
	} {
		loop.Prompt = "p"
		loop.Schedule = loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}
		loop.Runtime = loops.RuntimeACP
		loop.CreatedAt = now
		loop.UpdatedAt = now
		if err := store.SaveLoop(loop); err != nil {
			t.Fatal(err)
		}
	}

	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 100); err != nil {
		t.Fatalf("downgrade 0101 failed: %v", err)
	}
	const pinned = "pinnedpinnedpinnedpinned00"
	const revenue = "revenuerevenuerevenue00000"
	for _, stmt := range []string{
		`INSERT INTO boards (id, name, font_scale, created_at_ms, updated_at_ms) VALUES ('board-ops', 'Ops', 1.25, 1, 2)`,
		// A loop that still holds pre-app HTML, one whose HTML already became
		// an app, and one that was deleted.
		`INSERT INTO widgets (id, loop_id, title, current_version, size_hint, last_error, last_layout, published_run_id, created_at_ms, updated_at_ms)
		 VALUES ('w-html', 'loop-html', 'Inbox zero', 2, '3x2', 'boom', '{"clipped":1}', 'run-9', 3, 4),
		        ('w-app', 'loop-app', 'Revenue', 0, '', NULL, '', '', 3, 4),
		        ('w-gone', 'loop-gone', 'Gone', 1, '', NULL, '', '', 3, 4)`,
		`INSERT INTO widget_versions (widget_id, version, html, created_at_ms) VALUES ('w-html', 1, '<p>v1</p>', 3), ('w-html', 2, '<p>v2</p>', 4), ('w-gone', 1, '<p>gone</p>', 4)`,
		`INSERT INTO apps (id, title, current_version, loop_id, created_at_ms, updated_at_ms)
		 VALUES ('` + pinned + `', 'Notes', 1, '', 5, 6), ('` + revenue + `', 'Revenue', 1, 'loop-app', 5, 6)`,
		`INSERT INTO app_versions (app_id, version, bundle_sha256, entry_path, storage_dir, created_at_ms)
		 VALUES ('` + pinned + `', 1, 'sha-notes', 'index.html', '/tmp/notes', 5), ('` + revenue + `', 1, 'sha-revenue', 'index.html', '/tmp/revenue', 5)`,
		`INSERT INTO app_kv (app_id, scope_type, scope_owner_id, key, value_text, updated_at_ms) VALUES ('` + pinned + `', 'personal', 'local', 'k', 'v', 6)`,
		`INSERT INTO board_widgets (board_id, widget_id, app_id, x, y, w, h, placed_by, created_at_ms, updated_at_ms)
		 VALUES ('board-ops', 'w-html', NULL, 0, 0, 3, 2, 'user', 7, 8),
		        ('board-ops', 'w-app', NULL, 3, 0, 2, 2, 'llm', 7, 8),
		        ('board-ops', 'w-gone', NULL, 0, 2, 2, 2, 'llm', 7, 8),
		        ('board-ops', 'app_` + pinned + `', '` + pinned + `', 0, 4, 3, 3, 'user', 7, 8)`,
	} {
		if _, err := store.db.Exec(stmt); err != nil {
			t.Fatalf("seed %s: %v", stmt, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatalf("upgrade to boards as apps failed: %v", err)
	}

	boards, err := store.ListApps(ctx, storage.LocalWorkspaceID, storage.AppListFilter{Kind: storage.AppKindBoard, Query: "Ops"})
	if err != nil || len(boards) != 1 || boards[0].CurrentVersion != 0 || !boards[0].CreatedAt.Equal(time.UnixMilli(1).UTC()) {
		t.Fatalf("migrated board = %+v, %v", boards, err)
	}
	boardID := boards[0].ID
	if scale, err := store.BoardFontScale(ctx, boardID); err != nil || scale != 1.25 {
		t.Fatalf("font scale = %v, %v", scale, err)
	}
	tiles, err := store.ListBoardTiles(ctx, boardID)
	if err != nil || len(tiles) != 3 {
		t.Fatalf("tiles = %+v, %v", tiles, err)
	}
	html, app, notes := tiles[0], tiles[1], tiles[2]
	if html.Loop == nil || html.Loop.ID != "loop-html" || html.App.Title != "Inbox zero" || html.App.CurrentVersion != 0 || html.W != 3 || html.H != 2 || html.PlacedBy != storage.TilePlacedByUser || html.LastError != "boom" {
		t.Fatalf("tile of a loop with pre-app HTML = %+v", html)
	}
	if app.Loop == nil || app.Loop.ID != "loop-app" || app.App.ID != revenue || app.App.CurrentVersion != 1 || app.App.BundleSHA256 != "sha-revenue" || app.X != 3 {
		t.Fatalf("tile of a loop that already had its app = %+v", app)
	}
	if notes.Loop != nil || notes.App.ID != pinned || notes.Y != 4 || notes.W != 3 || notes.H != 3 {
		t.Fatalf("pinned app tile = %+v", notes)
	}
	loopApp, err := store.GetLoopApp(ctx, "loop-html")
	if err != nil || loopApp.AppID != html.App.ID || loopApp.SizeHint != "3x2" || loopApp.LastLayout != `{"clipped":1}` || loopApp.PublishedRunID != "run-9" {
		t.Fatalf("loop app = %+v, %v", loopApp, err)
	}
	legacy, err := store.ListLegacyWidgetHTML(ctx)
	if err != nil || len(legacy) != 1 || legacy[0].AppID != html.App.ID || legacy[0].HTML != "<p>v2</p>" || legacy[0].Title != "Inbox zero" {
		t.Fatalf("pre-app HTML awaiting conversion = %+v, %v", legacy, err)
	}
	if _, err := store.GetLoopApp(ctx, "loop-gone"); !errors.Is(err, storage.ErrAppNotFound) {
		t.Fatalf("deleted loop kept an app: %v", err)
	}
	if entry, err := store.GetAppKV(ctx, pinned, storage.AppKVScopePersonal, storage.AppKVLocalOwnerID, "k"); err != nil || entry.Value != "v" {
		t.Fatalf("app storage = %+v, %v", entry, err)
	}
	// Loop apps that never published stay out of the library.
	library, err := store.ListApps(ctx, storage.LocalWorkspaceID, storage.AppListFilter{Kind: storage.AppKindArtifact})
	if err != nil || len(library) != 2 {
		t.Fatalf("library = %+v, %v", library, err)
	}
}
