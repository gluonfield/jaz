package storage_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type goalStore interface {
	storage.Store
	StartSessionTurn(string, storage.Turn, ...storage.Message) error
}

func TestGoalClearSurvivesStaleSessionSaves(t *testing.T) {
	for _, adapter := range []string{"sqlite", "json"} {
		t.Run(adapter, func(t *testing.T) {
			var store goalStore
			if adapter == "sqlite" {
				db, err := sqlitestore.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					db.Close()
				})
				store = db
			} else {
				files, err := jsonstore.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				store = files
			}
			session, err := store.CreateSession(storage.CreateSession{Slug: "goal"})
			if err != nil {
				t.Fatal(err)
			}
			turn := storage.Turn{PlanRequested: true, GoalRequested: true, ActiveOperation: "compact"}
			if err := store.StartSessionTurn(session.ID, turn); err != nil {
				t.Fatal(err)
			}
			state := &goal.State{Identity: goal.Identity{ID: "first", Objective: "Finish the work", Status: goal.StatusActive}}
			if _, err := store.UpdateSessionGoal(session.ID, nil, state); err != nil {
				t.Fatal(err)
			}
			stale, err := store.LoadSession(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, err := store.UpdateSessionGoal(session.ID, nil, nil); err != nil {
					t.Fatal(err)
				}
			}
			stale.Title = "A concurrent metadata update"
			if err := store.SaveSession(stale); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.LoadSession(session.ID)
			turn.GoalRequested = false
			if loaded.Turn != nil {
				turn.StartedAt = loaded.Turn.StartedAt
			}
			if err != nil || loaded.Goal != nil || loaded.Turn == nil || !reflect.DeepEqual(*loaded.Turn, turn) || loaded.Status != storage.StatusRunning || loaded.Title != stale.Title {
				t.Fatalf("stale save restored goal state: %+v, %v", loaded, err)
			}
			if _, err := store.UpdateSessionGoal(session.ID, state, state); !errors.Is(err, storage.ErrGoalChanged) {
				t.Fatalf("late goal update = %v", err)
			}
			if _, err := store.UpdateSessionGoal(session.ID, nil, state); !errors.Is(err, storage.ErrGoalNotRequested) {
				t.Fatalf("late goal creation = %v", err)
			}
			if err := store.CompleteSession(session.ID, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdateSessionGoal(session.ID, nil, state); !errors.Is(err, storage.ErrGoalNotRequested) {
				t.Fatalf("goal creation after the cleared turn finished = %v", err)
			}
			turn.GoalRequested = true
			if err := store.StartSessionTurn(session.ID, turn); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdateSessionGoal(session.ID, nil, state); err != nil {
				t.Fatalf("explicit goal reactivation = %v", err)
			}
		})
	}
}
