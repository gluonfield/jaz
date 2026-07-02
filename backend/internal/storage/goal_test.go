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

func TestGoalProjectionFromEventAcceptsJazSnapshot(t *testing.T) {
	projection, ok, err := GoalProjectionFromEvent(sessionevents.Event{
		Type: sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{
			Identity: goal.Identity{
				Objective: "source-less objective",
				Status:    goal.StatusActive,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !projection.Seen || projection.State == nil || projection.State.Objective != "source-less objective" {
		t.Fatalf("projection = %#v, ok = %v", projection, ok)
	}
}

func TestGoalProjectionFromEventClearsRequestOnlySnapshot(t *testing.T) {
	projection, ok, err := GoalProjectionFromEvent(sessionevents.Event{
		Type: sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{
			Identity: goal.Identity{
				Objective: "user prompt text",
				Status:    goal.StatusRequested,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !projection.Seen || projection.State != nil {
		t.Fatalf("projection = %#v, ok = %v", projection, ok)
	}
}

func TestGoalDisplayEventsConvertsRequestOnlySnapshotToClear(t *testing.T) {
	events := GoalDisplayEvents([]sessionevents.Event{
		{
			SessionID: "thread-1",
			Type:      sessionevents.TypeGoalUpdate,
			Goal: &sessionevents.GoalEvent{
				Identity: goal.Identity{
					Objective: "user prompt text",
					Status:    goal.StatusRequested,
				},
			},
		},
	})
	if len(events) != 1 || events[0].Type != sessionevents.TypeGoalClear || events[0].Goal != nil {
		t.Fatalf("events = %#v, want goal_clear", events)
	}
}

func TestUnmarshalGoalStateIgnoresRequestOnlySnapshot(t *testing.T) {
	state, err := UnmarshalGoalState(`{"objective":"user prompt text","status":"requested"}`)
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatalf("state = %#v, want nil", state)
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
