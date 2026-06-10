package sqlite

import (
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var sqliteMigrations embed.FS

var gooseMu sync.Mutex

func (s *Store) migrate() error {
	if err := runMigrations(s.db); err != nil {
		return err
	}
	_, err := s.db.Exec(`PRAGMA user_version = 4`)
	return err
}

func runMigrations(db *sql.DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()
	goose.SetBaseFS(sqliteMigrations)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set sqlite migration dialect: %w", err)
	}
	// Pre-goose thread tables lack columns that later migrations reference
	// (0006 updates input_tokens), so add them before goose runs.
	if err := ensureLegacyThreadColumns(db); err != nil {
		return err
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("run sqlite migrations: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_threads_source ON threads(source_type, source_id)`); err != nil {
		return fmt.Errorf("create source thread index: %w", err)
	}
	return nil
}

type columnMigration struct {
	Name       string
	Definition string
}

func ensureLegacyThreadColumns(db *sql.DB) error {
	columns, err := tableColumns(db, "threads")
	if err != nil {
		return err
	}
	// No threads table at all means a fresh database; goose creates the
	// full schema and there is nothing legacy to patch.
	if len(columns) == 0 {
		return nil
	}
	for _, column := range []columnMigration{
		{Name: "archived", Definition: "archived INTEGER NOT NULL DEFAULT 0"},
		{Name: "error", Definition: "error TEXT"},
		{Name: "cwd", Definition: "cwd TEXT"},
		{Name: "input_tokens", Definition: "input_tokens INTEGER NOT NULL DEFAULT 0"},
		{Name: "cached_input_tokens", Definition: "cached_input_tokens INTEGER NOT NULL DEFAULT 0"},
		{Name: "output_tokens", Definition: "output_tokens INTEGER NOT NULL DEFAULT 0"},
		{Name: "reasoning_output_tokens", Definition: "reasoning_output_tokens INTEGER NOT NULL DEFAULT 0"},
		{Name: "total_tokens", Definition: "total_tokens INTEGER NOT NULL DEFAULT 0"},
		{Name: "queued_messages", Definition: "queued_messages TEXT NOT NULL DEFAULT '[]'"},
		{Name: "source_type", Definition: "source_type TEXT"},
		{Name: "source_id", Definition: "source_id TEXT"},
	} {
		if columns[column.Name] {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE threads ADD COLUMN ` + column.Definition); err != nil {
			return fmt.Errorf("add legacy thread column %s: %w", column.Name, err)
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, fmt.Errorf("read %s columns: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}
