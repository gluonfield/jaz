package acp

import (
	"encoding/json"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestPermissionPlanContentExtractsPlan(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			name: "rawInput plan",
			raw:  map[string]any{"toolCallId": "t1", "kind": "switch_mode", "rawInput": map[string]any{"plan": "Step one.\nStep two."}},
			want: "Step one.\nStep two.",
		},
		{
			name: "content block fallback",
			raw: map[string]any{"toolCallId": "t2", "kind": "switch_mode", "content": []any{
				map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "Plan body."}},
			}},
			want: "Plan body.",
		},
		{
			name: "non switch_mode ignored",
			raw:  map[string]any{"toolCallId": "t3", "kind": "edit", "rawInput": map[string]any{"plan": "nope"}},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var call acpschema.ToolCallUpdate
			if err := json.Unmarshal(mustJSON(t, tc.raw), &call); err != nil {
				t.Fatal(err)
			}
			if got := permissionPlanContent(call); got != tc.want {
				t.Fatalf("permissionPlanContent = %q, want %q", got, tc.want)
			}
		})
	}
}

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
	manager.recordAndPublishDirect(sessionevents.Event{
		SessionID: session.ID,
		Type:      "acp_tool",
		ACP:       &sessionevents.ACPEvent{ID: session.ID, Slug: "main", Title: "a very long first prompt", Agent: "codex", Modes: modes},
	})
	manager.recordAndPublishDirect(sessionevents.Event{
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

func TestTranscriptChunksFlushBeforeStatus(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "chunks", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	live := manager.Events.Subscribe(t.Context(), session.ID)

	job := &jobState{
		Job: Job{
			ID:         session.ID,
			Slug:       session.Slug,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			State:      StateRunning,
			Assistant:  "Hel",
		},
	}
	manager.queueACPMessage(job, "Hel")
	job.Assistant = "Hello"
	manager.queueACPMessage(job, "lo")
	job.State = StateIdle
	manager.publishACPStatus(job.Snapshot())

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored %d events, want 2: %#v", len(stored), stored)
	}
	if stored[0].Type != "acp_message" || stored[0].Content != "Hello" || stored[0].Seq != 1 {
		t.Fatalf("message event = %#v", stored[0])
	}
	if stored[1].Type != "acp" || stored[1].ACP.State != StateIdle || stored[1].Seq != 2 {
		t.Fatalf("status event = %#v", stored[1])
	}
	state, err := store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Assistant != "Hello" || state.State != StateIdle {
		t.Fatalf("state = %#v", state)
	}

	first := receiveLiveEvent(t, live)
	second := receiveLiveEvent(t, live)
	if first.Type != "acp_message" || first.Content != "Hello" || first.Seq != 1 {
		t.Fatalf("first live event = %#v", first)
	}
	if second.Type != "acp" || second.ACP.State != StateIdle || second.Seq != 2 {
		t.Fatalf("second live event = %#v", second)
	}
	assertTranscriptBufferIdle(t, manager, session.ID)
}

func TestTranscriptChunksFlushOnTimerWhileRunning(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "timer-chunks", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	live := manager.Events.Subscribe(t.Context(), session.ID)

	job := &jobState{
		Job: Job{
			ID:         session.ID,
			Slug:       session.Slug,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			State:      StateRunning,
			Assistant:  "live text",
		},
	}
	manager.jobsByID[session.ID] = job
	manager.queueACPMessage(job, "live ")
	manager.queueACPMessage(job, "text")

	event := receiveLiveEvent(t, live)
	if event.Type != "acp_message" || event.Content != "live text" || event.ACP.State != StateRunning {
		t.Fatalf("live event = %#v", event)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Type != "acp_message" || stored[0].Content != "live text" {
		t.Fatalf("stored events = %#v", stored)
	}
	state, err := store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != StateRunning || state.Assistant != "live text" {
		t.Fatalf("state = %#v", state)
	}
	assertTranscriptBufferIdle(t, manager, session.ID)
}

func assertTranscriptBufferIdle(t *testing.T, manager *Manager, sessionID string) {
	t.Helper()
	buffer := manager.transcriptBuffers.get(sessionID, false)
	if buffer == nil {
		return
	}
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if len(buffer.runs) != 0 || buffer.timer != nil {
		t.Fatalf("transcript buffer retained state: runs=%d timer=%v", len(buffer.runs), buffer.timer != nil)
	}
}

func receiveLiveEvent(t *testing.T, ch <-chan sessionevents.Event) sessionevents.Event {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event")
	}
	return sessionevents.Event{}
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
	job := &jobState{
		Job: Job{
			ID:         session.ID,
			Slug:       session.Slug,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			State:      StateCancelled,
			LastToolAt: lastTool,
		},
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
