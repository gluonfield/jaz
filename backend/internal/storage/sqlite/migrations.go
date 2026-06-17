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
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("run sqlite migrations: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_threads_source ON threads(source_type, source_id)`); err != nil {
		return fmt.Errorf("create source thread index: %w", err)
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
