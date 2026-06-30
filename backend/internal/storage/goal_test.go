package storage

import (
	"testing"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestGoalProjectionFromEventRequiresCompleteSnapshot(t *testing.T) {
	_, ok, err := GoalProjectionFromEvent(sessionevents.Event{
		Type: sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{
			Identity: goal.Identity{Status: goal.StatusActive},
			Progress: goal.Progress{ProgressMessage: "still working"},
		},
	})
	if !ok {
		t.Fatalf("goal event was not recognized")
	}
	if err == nil {
		t.Fatalf("partial goal update was accepted")
	}
}

func TestGoalProjectionFromEventAcceptsCompleteSnapshot(t *testing.T) {
	projection, ok, err := GoalProjectionFromEvent(sessionevents.Event{
		Type: sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{
			Identity: goal.Identity{
				Objective: "ship it",
				Status:    goal.StatusActive,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !projection.Seen || projection.State == nil ||
		projection.State.Objective != "ship it" ||
		projection.State.Status != goal.StatusActive {
		t.Fatalf("projection = %#v, ok = %v", projection, ok)
	}
}

func TestGoalProjectionFromEventClearsSnapshot(t *testing.T) {
	projection, ok, err := GoalProjectionFromEvent(sessionevents.Event{Type: sessionevents.TypeGoalClear})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !projection.Seen || projection.State != nil {
		t.Fatalf("projection = %#v, ok = %v", projection, ok)
	}
}
