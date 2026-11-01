package bots

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBotHomeMigrationOnlyMovesLegacyInstructionsOnce(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	newHome := func() string {
		t.Helper()
		home := t.TempDir()
		bot, err := store.CreateSession(storage.CreateSession{
			SourceType: storage.SourceBot,
			RuntimeRef: &storage.RuntimeRef{Cwd: home},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveBot(storage.BotRecord{ThreadID: bot.ID, Kind: KindBot}); err != nil {
			t.Fatal(err)
		}
		return home
	}
	legacy, scoped, empty := newHome(), newHome(), newHome()
	write(t, filepath.Join(legacy, "AGENTS.md"), "Legacy coordinator rules.")
	write(t, filepath.Join(scoped, instructionsFile), "Scoped coordinator rules.")
	write(t, filepath.Join(scoped, "AGENTS.md"), "Shared workspace rules.")
	if err := MigrateHomes(store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("legacy identity is still inherited by workers: %v", err)
	}
	write(t, filepath.Join(empty, "AGENTS.md"), "New workspace rules.")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	store = reopened
	createdLater := newHome()
	write(t, filepath.Join(createdLater, "AGENTS.md"), "Later workspace rules.")
	if err := MigrateHomes(store); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		filepath.Join(legacy, instructionsFile):  "Legacy coordinator rules.",
		filepath.Join(scoped, instructionsFile):  "Scoped coordinator rules.",
		filepath.Join(scoped, "AGENTS.md"):       "Shared workspace rules.",
		filepath.Join(empty, "AGENTS.md"):        "New workspace rules.",
		filepath.Join(createdLater, "AGENTS.md"): "Later workspace rules.",
	} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("instructions at %s changed: %q, %v", path, data, err)
		}
	}
}
