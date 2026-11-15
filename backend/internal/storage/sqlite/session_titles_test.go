package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestAgentTitlesOverrideGeneratedTitlesAndPreserveManualTitles(t *testing.T) {
	for _, backend := range []string{"sqlite", "json"} {
		t.Run(backend, func(t *testing.T) {
			var store storage.SessionStore
			if backend == "sqlite" {
				db, err := New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				store = db
			} else {
				db, err := jsonstore.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				store = db
			}
			for _, initiallyLocked := range []bool{false, true} {
				session, err := store.CreateSession(storage.CreateSession{Title: "Placeholder"})
				if err != nil {
					t.Fatal(err)
				}
				if initiallyLocked {
					generated, updated, err := store.UpdateGeneratedSessionTitle(session.ID, "Generated")
					if err != nil || !updated || !generated.TitleLocked || generated.ManualTitle {
						t.Fatalf("generated title = %+v updated=%v err=%v", generated, updated, err)
					}
				}
				agent, updated, err := store.UpdateSessionTitleFromAgent(session.ID, "Agent chosen")
				if err != nil || !updated || agent.Title != "Agent chosen" || !agent.TitleLocked || agent.ManualTitle {
					t.Fatalf("agent title = %+v updated=%v err=%v", agent, updated, err)
				}
				generated, updated, err := store.UpdateGeneratedSessionTitle(session.ID, "Late generator")
				if err != nil || updated || generated.Title != "Agent chosen" || !generated.TitleLocked || generated.ManualTitle {
					t.Fatalf("late generator overwrote agent title: %+v updated=%v err=%v", generated, updated, err)
				}
			}
			manual, err := store.CreateSession(storage.CreateSession{Title: "Original"})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateSessionTitle(manual.ID, "User chosen"); err != nil {
				t.Fatal(err)
			}
			for _, update := range []func(string, string) (storage.Session, bool, error){store.UpdateSessionTitleFromAgent, store.UpdateGeneratedSessionTitle} {
				kept, updated, err := update(manual.ID, "Unwanted replacement")
				if err != nil || updated || kept.Title != "User chosen" || !kept.ManualTitle || kept.TitleLocked {
					t.Fatalf("manual title changed: %+v updated=%v err=%v", kept, updated, err)
				}
			}
		})
	}
}
