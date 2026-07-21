package storage

import (
	"testing"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestDisplayEventsConvertsRequestOnlyGoalSnapshotToClear(t *testing.T) {
	events := DisplayEvents([]sessionevents.Event{{
		SessionID: "thread-1",
		Type:      sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{Identity: goal.Identity{
			Objective: "user prompt text",
			Status:    goal.StatusRequested,
		}},
	}})
	if len(events) != 1 || events[0].Type != sessionevents.TypeGoalClear || events[0].Goal != nil {
		t.Fatalf("events = %#v, want goal_clear", events)
	}
}

func TestDisplayEventsHideLegacyCodexTransportFallback(t *testing.T) {
	events := DisplayEvents([]sessionevents.Event{{
		Type:    sessionevents.TypeACPMessage,
		Content: "Falling back from WebSockets to HTTPS transport. disconnected",
		ACP:     &sessionevents.ACPEvent{TextRunID: "message:codex:warning:turn:1"},
	}})
	if len(events) != 0 {
		t.Fatalf("events = %#v, want transport fallback hidden", events)
	}
}

func TestDisplayEventsHideLegacyCodexModelMetadataWarning(t *testing.T) {
	events := DisplayEvents([]sessionevents.Event{{
		Type:    sessionevents.TypeACPMessage,
		Content: "Model metadata for `qwen3.8-max-preview` not found. Defaulting to fallback metadata; this can degrade performance and cause issues.",
		ACP:     &sessionevents.ACPEvent{TextRunID: "message:codex:warning:turn:1"},
	}})
	if len(events) != 0 {
		t.Fatalf("events = %#v, want metadata warning hidden", events)
	}
}

func TestDisplayEventsKeepOtherCodexWarnings(t *testing.T) {
	events := DisplayEvents([]sessionevents.Event{{
		Type:    sessionevents.TypeACPMessage,
		Content: "Approval interrupted",
		ACP:     &sessionevents.ACPEvent{TextRunID: "message:codex:warning:turn:1"},
	}})
	if len(events) != 1 || events[0].Content != "Approval interrupted" {
		t.Fatalf("events = %#v, want warning retained", events)
	}
}
