package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionGoalRoundTripAndMirror(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "goal"})
	if err != nil {
		t.Fatal(err)
	}
	budget := int64(1000)
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{
			Identity: goal.Identity{
				Objective: "Ship visible goal state",
				Status:    goal.StatusActive,
			},
			Budget: goal.Budget{
				TokenBudget: &budget,
				TokensUsed:  250,
			},
			TimeUsedSeconds: 42,
		},
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal == nil || loaded.Goal.Objective != "Ship visible goal state" ||
		loaded.Goal.Status != goal.StatusActive ||
		loaded.Goal.RemainingTokens == nil || *loaded.Goal.RemainingTokens != 750 ||
		loaded.Goal.TimeUsedSeconds != 42 {
		t.Fatalf("goal = %#v", loaded.Goal)
	}
	mirror, err := jsonstore.New(store.RootDir())
	if err != nil {
		t.Fatal(err)
	}
	mirrored, err := mirror.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mirrored.Goal == nil || mirrored.Goal.Objective != loaded.Goal.Objective ||
		mirrored.Goal.RemainingTokens == nil || *mirrored.Goal.RemainingTokens != 750 ||
		mirrored.Goal.TimeUsedSeconds != 42 {
		t.Fatalf("mirrored goal = %#v, want %#v", mirrored.Goal, loaded.Goal)
	}

	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: sessionevents.TypeGoalClear}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal != nil {
		t.Fatalf("goal after clear = %#v", loaded.Goal)
	}
	mirrored, err = mirror.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mirrored.Goal != nil {
		t.Fatalf("mirrored goal after clear = %#v", mirrored.Goal)
	}
}
