package acp

import (
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

// The stored copy of an event must not repeat session-constant fields (title,
// slug, mode catalog) — they dominated transcript payloads. The live copy
// keeps them so subscribers can label sessions they haven't fetched yet.
func TestRecordAndPublishSlimsStoredCopy(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, Events: sessionevents.New()}
	live := manager.Events.Subscribe(t.Context(), session.ID)

	modes := sessionevents.ACPModeState{
		CurrentModeID: "plan",
		PlanModeID:    "plan",
		AvailableModes: []sessionevents.ACPMode{
			{ID: "plan", Name: "Plan", Description: "long catalog text"},
		},
	}
	manager.recordAndPublish(sessionevents.Event{
		SessionID: session.ID,
		Type:      "acp_tool",
		ACP:       &sessionevents.ACPEvent{ID: session.ID, Slug: "main", Title: "a very long first prompt", Agent: "codex", Modes: modes},
	})
	manager.recordAndPublish(sessionevents.Event{
		SessionID: session.ID,
		Type:      "acp",
		ACP: &sessionevents.ACPEvent{
			ID: session.ID, Slug: "main", Title: "a very long first prompt", Agent: "codex",
			Modes: modes,
			Plan:  []sessionevents.PlanEntry{{Content: "step"}},
		},
	})

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored %d events, want 2", len(stored))
	}
	for _, event := range stored {
		if event.ACP.Title != "" || len(event.ACP.Modes.AvailableModes) != 0 {
			t.Fatalf("stored event still carries envelope: %+v", event.ACP)
		}
		// The slug survives as a durable label fallback.
		if event.ACP.Slug != "main" {
			t.Fatalf("stored event lost its slug: %+v", event.ACP)
		}
	}
	if stored[0].ACP.Modes.CurrentModeID != "" {
		t.Fatalf("plan-less event should drop modes entirely, got %+v", stored[0].ACP.Modes)
	}
	// Plan approval reads current/plan mode ids from the event.
	if stored[1].ACP.Modes.CurrentModeID != "plan" || stored[1].ACP.Modes.PlanModeID != "plan" {
		t.Fatalf("plan-bearing event lost its mode ids: %+v", stored[1].ACP.Modes)
	}

	select {
	case event := <-live:
		if event.ACP.Title != "a very long first prompt" || len(event.ACP.Modes.AvailableModes) != 1 {
			t.Fatalf("published copy should keep labels: %+v", event.ACP)
		}
		if event.Seq != stored[0].Seq {
			t.Fatalf("published seq %d, stored seq %d", event.Seq, stored[0].Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("no live event published")
	}
}

func TestInactiveStatusClearsStoredPermissions(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "inactive",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Agent:     AgentCodex,
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	lastTool := time.Now().UTC().Add(-time.Minute)
	if err := store.SaveACPState(session.ID, storage.ACPState{
		ID:         session.ID,
		ACPAgent:   AgentCodex,
		ACPSession: "acp-session",
		State:      StateRunning,
		Permissions: []sessionevents.ACPPermission{{
			ID: "approval-1",
		}},
		ToolCalls: []sessionevents.ACPToolCall{{
			ID:     "tool-1",
			Title:  "go test ./...",
			Status: "in_progress",
		}},
		LastEventAt: lastTool,
		LastToolAt:  lastTool,
	}); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(store, Config{}, nil)
	status, err := manager.Status(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateNotRunning {
		t.Fatalf("state = %q, want %q", status.State, StateNotRunning)
	}
	if len(status.Permissions) != 0 {
		t.Fatalf("inactive status kept stale permissions: %#v", status.Permissions)
	}
	if len(status.ToolCalls) != 1 || status.LastToolAt.IsZero() {
		t.Fatalf("inactive diagnostics lost tool state: %#v", status)
	}
}

func TestResolveDanglingToolCallsDoesNotRefreshLastToolAt(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "cancelled", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	lastTool := time.Now().UTC().Add(-5 * time.Minute)
	job := &Job{
		ID:         session.ID,
		Slug:       session.Slug,
		ACPAgent:   AgentCodex,
		ACPSession: "acp-session",
		State:      StateCancelled,
		LastToolAt: lastTool,
		toolByID: map[string]sessionevents.ACPToolCall{
			"tool-1": {ID: "tool-1", Title: "go test ./...", Status: "in_progress"},
		},
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()

	manager.resolveDanglingToolCalls(job)

	if !job.LastToolAt.Equal(lastTool) {
		t.Fatalf("LastToolAt = %s, want unchanged %s", job.LastToolAt, lastTool)
	}
	if got := job.toolByID["tool-1"].Status; got != "cancelled" {
		t.Fatalf("tool status = %q, want cancelled", got)
	}
	if job.LastEventAt.IsZero() {
		t.Fatal("LastEventAt was not updated for cleanup event")
	}
}
