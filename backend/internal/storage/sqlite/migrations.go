package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var sqliteMigrations embed.FS

func (s *Store) migrate() error {
	if err := runMigrations(s.db); err != nil {
		return err
	}
	_, err := s.db.Exec(`PRAGMA user_version = 4`)
	return err
}

func runMigrations(db *sql.DB) error {
	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("load sqlite migrations: %w", err)
	}
	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		db,
		migrations,
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return fmt.Errorf("create sqlite migration provider: %w", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
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
