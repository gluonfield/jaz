package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// Every historical version-34 schema must converge to a threads table with
// `unread` and without `last_seen_at_ms`, and re-running must not fail.
func TestThreadUnreadReconcileHandlesLegacyStates(t *testing.T) {
	cases := []struct{ name, create string }{
		{"legacy_last_seen_no_unread", `CREATE TABLE threads (id TEXT, last_seen_at_ms INTEGER NOT NULL DEFAULT 0)`},
		{"already_unread_no_last_seen", `CREATE TABLE threads (id TEXT, unread INTEGER NOT NULL DEFAULT 0)`},
		{"both_columns", `CREATE TABLE threads (id TEXT, last_seen_at_ms INTEGER NOT NULL DEFAULT 0, unread INTEGER NOT NULL DEFAULT 0)`},
		{"neither", `CREATE TABLE threads (id TEXT)`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(tc.create); err != nil {
				t.Fatal(err)
			}
			for pass := 0; pass < 2; pass++ {
				if err := upThreadUnreadReconcile(context.Background(), db); err != nil {
					t.Fatalf("reconcile pass %d: %v", pass, err)
				}
			}
			cols, err := tableColumns(db, "threads")
			if err != nil {
				t.Fatal(err)
			}
			if !cols["unread"] {
				t.Fatal("unread missing after reconcile")
			}
			if cols["last_seen_at_ms"] {
				t.Fatal("last_seen_at_ms should be dropped")
			}
		})
	}
}

func TestFreshMigrationsAddUnreadColumn(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cols, err := tableColumns(store.db, "threads")
	if err != nil {
		t.Fatal(err)
	}
	if !cols["unread"] {
		t.Fatal("fresh migration did not add threads.unread")
	}
	if cols["last_seen_at_ms"] {
		t.Fatal("fresh migration left an obsolete last_seen_at_ms")
	}
}
