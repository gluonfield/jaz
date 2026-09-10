package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestModelSelectionOwnsContextLimit(t *testing.T) {
	sqlite, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer sqlite.Close()
	json, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, store := range map[string]storage.SessionStore{"sqlite": sqlite, "json": json} {
		t.Run(name, func(t *testing.T) {
			session, err := store.CreateSession(storage.CreateSession{Slug: "model-context", Model: "first", Runtime: storage.RuntimeACP})
			if err != nil {
				t.Fatal(err)
			}
			session.Usage = storage.Usage{ContextWindowTokens: 200000, ContextTokens: 1234, InputTokens: 1234}
			if err := store.SaveSession(session); err != nil {
				t.Fatal(err)
			}
			for _, selection := range []struct {
				model  string
				effort string
				window int64
			}{{"first", "high", 200000}, {"second", "high", 0}} {
				if err := store.UpdateSessionModel(session.ID, selection.model, selection.effort); err != nil {
					t.Fatal(err)
				}
				loaded, err := store.LoadSession(session.ID)
				if err != nil {
					t.Fatal(err)
				}
				if loaded.Model != selection.model || loaded.ReasoningEffort != selection.effort || loaded.Usage.ContextWindowTokens != selection.window || loaded.Usage.ContextTokens != 1234 || loaded.Usage.InputTokens != 1234 {
					t.Fatalf("model selection = %#v, usage = %#v", loaded, loaded.Usage)
				}
			}
		})
	}
}
