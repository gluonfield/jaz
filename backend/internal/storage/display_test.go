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

// The app-server adapter prefixes warnings with "Warning: " and sends no
// message id; the older adapter sent bare text under a "codex:warning:" id.
func TestDisplayEventsHideCodexTransportFallbackInBothWireShapes(t *testing.T) {
	for _, event := range []sessionevents.Event{
		{
			Type:    sessionevents.TypeACPMessage,
			Content: "Warning: Falling back from WebSockets to HTTPS transport. disconnected\n\n",
			ACP:     &sessionevents.ACPEvent{TextRunID: "turn:1786548004124129000:1"},
		},
		{
			Type:    sessionevents.TypeACPMessage,
			Content: "Falling back from WebSockets to HTTPS transport. disconnected",
			ACP:     &sessionevents.ACPEvent{TextRunID: "message:codex:warning:turn:1"},
		},
	} {
		if events := DisplayEvents([]sessionevents.Event{event}); len(events) != 0 {
			t.Fatalf("events = %#v, want transport fallback hidden", events)
		}
	}
}

// The fallback-metadata warning explains a degraded session, so it stays visible.
func TestDisplayEventsKeepCodexModelMetadataWarning(t *testing.T) {
	content := "Warning: Model metadata for `gpt-5.6-sol` not found. Defaulting to fallback metadata; this can degrade performance and cause issues.\n\n"
	events := DisplayEvents([]sessionevents.Event{{
		Type:    sessionevents.TypeACPMessage,
		Content: content,
		ACP:     &sessionevents.ACPEvent{TextRunID: "turn:1786548004124129000:1"},
	}})
	if len(events) != 1 || events[0].Content != content {
		t.Fatalf("events = %#v, want metadata warning retained", events)
	}
}

func TestDisplayEventsKeepOtherCodexWarnings(t *testing.T) {
	events := DisplayEvents([]sessionevents.Event{{
		Type:    sessionevents.TypeACPMessage,
		Content: "Warning: Approval interrupted",
		ACP:     &sessionevents.ACPEvent{TextRunID: "turn:1786548004124129000:1"},
	}})
	if len(events) != 1 || events[0].Content != "Warning: Approval interrupted" {
		t.Fatalf("events = %#v, want warning retained", events)
	}
}
