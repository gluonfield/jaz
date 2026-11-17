package storage_test

import (
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestQuestionReadersKeepThreadScopeAndResolution(t *testing.T) {
	for _, kind := range []string{"sqlite", "json"} {
		t.Run(kind, func(t *testing.T) {
			var store storage.Store
			var err error
			if kind == "sqlite" {
				db, openErr := sqlite.New(t.TempDir())
				err = openErr
				if err == nil {
					t.Cleanup(func() { _ = db.Close() })
					store = db
				}
			} else {
				store, err = jsonstore.New(t.TempDir())
			}
			if err != nil {
				t.Fatal(err)
			}
			thread, err := store.CreateSession(storage.CreateSession{Slug: "questions"})
			if err != nil {
				t.Fatal(err)
			}
			other, err := store.CreateSession(storage.CreateSession{Slug: "other"})
			if err != nil {
				t.Fatal(err)
			}
			first := sessionevents.ACPPermission{ID: "first", Status: "pending", Questions: []sessionevents.ACPQuestion{{ID: "q", Question: "Choose"}}}
			second := first
			second.ID = "second"
			if err := store.AppendSessionEvents(thread.ID,
				sessionevents.Event{Type: "permission_request", Permission: &first},
				sessionevents.Event{Type: "note", Content: "Unrelated transcript payload"},
				sessionevents.Event{Type: "permission_request", Permission: &second},
				sessionevents.Event{Type: "permission_request", Permission: &sessionevents.ACPPermission{ID: "approval", Status: "pending"}},
			); err != nil {
				t.Fatal(err)
			}
			assertOpen := func(id string, want bool) {
				t.Helper()
				if got, err := store.HasOpenQuestion(id); err != nil || got != want {
					t.Fatalf("open=%v, want %v, %v", got, want, err)
				}
			}
			assertOpen(thread.ID, true)
			assertOpen(other.ID, false)
			first.Status = "selected"
			first.Answers = map[string][]string{"q": {"Chosen"}}
			second.Status = "pending"
			if err := store.AppendSessionEvents(thread.ID, sessionevents.Event{Type: "permission_response", Permission: &first}, sessionevents.Event{Type: "permission_response", Permission: &second}); err != nil {
				t.Fatal(err)
			}
			assertOpen(thread.ID, true)
			second.Status = "cancelled"
			second.Origin = "user_input"
			if err := store.AppendSessionEvents(thread.ID, sessionevents.Event{Type: "permission_response", Permission: &second}); err != nil {
				t.Fatal(err)
			}
			assertOpen(thread.ID, false)
			events, err := store.LoadSessionQuestionEvents(thread.ID)
			if err != nil || len(events) != 6 {
				t.Fatalf("permission events=%+v, %v", events, err)
			}
			questions := sessionevents.Questions(events)
			if len(questions) != 2 || questions[0].Answers["q"][0] != "Chosen" || questions[1].Origin != "user_input" {
				t.Fatalf("question projection=%+v", questions)
			}
		})
	}
}
