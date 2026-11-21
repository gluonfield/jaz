package sqlite

import (
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/search"
)

func TestEventSearchUpgradeAndMutationLifecycle(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "existing-answer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: sessionevents.TypeACPMessage, Content: "quartz",
		ACP: &sessionevents.ACPEvent{ID: session.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(t.Context(), 53); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	queries := NewSearchQueries(store)
	check := func(word string, count int) {
		t.Helper()
		rows, err := queries.SearchThreadMessages(t.Context(), search.SearchThreadMessagesParams{Match: word, Limit: 10, HiddenPrefixes: "[]"})
		if err != nil || len(rows) != count {
			t.Fatalf("search %q: %#v, %v", word, rows, err)
		}
	}
	check("quartz", 1)
	if _, err := store.db.Exec("UPDATE session_events SET content = 'ruby' WHERE thread_id = ?", session.ID); err != nil {
		t.Fatal(err)
	}
	check("quartz", 0)
	check("ruby", 1)
	if _, err := store.db.Exec("DELETE FROM session_events WHERE thread_id = ?", session.ID); err != nil {
		t.Fatal(err)
	}
	check("ruby", 0)
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: sessionevents.TypeACPMessage, Content: "sapphire", ACP: &sessionevents.ACPEvent{ID: session.ID},
	}); err != nil {
		t.Fatal(err)
	}
	check("sapphire", 1)
	if _, err := store.db.Exec("DELETE FROM threads WHERE id = ?", session.ID); err != nil {
		t.Fatal(err)
	}
	check("sapphire", 0)
	if _, err := store.db.Exec("INSERT INTO event_search_fts(event_search_fts, rank) VALUES ('integrity-check', 1)"); err != nil {
		t.Fatalf("event search index diverged from content: %v", err)
	}
}
