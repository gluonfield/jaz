package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationNoTxContext("0035_thread_unread_reconcile.go", upThreadUnreadReconcile, downThreadUnreadReconcile)
}

// An earlier revision of migration 34 shipped as `last_seen_at_ms` before being
// reworked to `unread`; goose tracks by version, so DBs that ran the old 34 skip
// the new one and never get `unread`. This reconciles every version-34 state:
// add `unread` when missing, drop the obsolete `last_seen_at_ms` when present.
func upThreadUnreadReconcile(ctx context.Context, db *sql.DB) error {
	columns, err := tableColumns(db, "threads")
	if err != nil {
		return err
	}
	if !columns["unread"] {
		if _, err := db.ExecContext(ctx, `ALTER TABLE threads ADD COLUMN unread INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add threads.unread: %w", err)
		}
	}
	if columns["last_seen_at_ms"] {
		if _, err := db.ExecContext(ctx, `ALTER TABLE threads DROP COLUMN last_seen_at_ms`); err != nil {
			return fmt.Errorf("drop threads.last_seen_at_ms: %w", err)
		}
	}
	return nil
}

func downThreadUnreadReconcile(context.Context, *sql.DB) error { return nil }
