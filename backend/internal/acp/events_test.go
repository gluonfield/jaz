package acp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/google/uuid"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestPermissionIDsRemainUniqueAcrossManagerLifetimes(t *testing.T) {
	first := newPermissionID()
	second := newPermissionID()
	if first == second {
		t.Fatalf("permission IDs collided: %q", first)
	}
	for _, id := range []string{first, second} {
		if _, err := uuid.Parse(strings.TrimPrefix(id, "perm-")); err != nil {
			t.Fatalf("permission ID %q is not restart-safe: %v", id, err)
		}
	}
}

type failOnceEventStore struct {
	Store
	fail bool
}

func (s *failOnceEventStore) AppendSessionEvents(id string, events ...sessionevents.Event) error {
	if s.fail {
		s.fail = false
		return errors.New("injected event append failure")
	}
	return s.Store.AppendSessionEvents(id, events...)
}

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
			Assistant:   "large accumulated answer",
			Thought:     "large accumulated reasoning",
			Modes:       modes,
			Plan:        []sessionevents.PlanEntry{{Content: "step"}},
			ToolCalls:   []sessionevents.ACPToolCall{{ID: "tool-1"}},
			Permissions: []sessionevents.ACPPermission{{ID: "permission-1"}},
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
	if stored[1].ACP.Assistant != "" || stored[1].ACP.Thought != "" || stored[1].ACP.ToolCalls != nil || stored[1].ACP.Permissions != nil {
		t.Fatalf("stored status event repeated transcript state: %+v", stored[1].ACP)
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

func TestRecordAndPublishCommitsProjectionStateWithEventAppend(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "projection-append", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(&failOnceEventStore{Store: store, fail: true}, Config{}, nil)
	text := func(content string) sessionevents.Event {
		return sessionevents.Event{
			SessionID: session.ID,
			Type:      sessionevents.TypeACPMessage,
			Content:   content,
			ACP:       &sessionevents.ACPEvent{ID: session.ID, TextRunID: "message:one", State: StateRunning},
		}
	}
	manager.recordAndPublishDirect(text("lost"))
	manager.recordAndPublishDirect(text("kept"))

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Content != "kept" || !strings.HasSuffix(stored[0].ProjectionKey, ":1") {
		t.Fatalf("projection advanced past failed append: %#v", stored)
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
	manager.queueACPMessageWithID(job, "Hel", "message-1")
	job.Assistant = "Hello"
	manager.queueACPMessageWithID(job, "lo", "message-1")
	job.State = StateIdle
	manager.publishACPStatus(job.eventView())

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
	manager.queueACPMessageWithID(job, "live ", "message-1")
	manager.queueACPMessageWithID(job, "text", "message-1")

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

func TestACPTranscriptTextRunSurvivesToolBarrier(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, Events: sessionevents.New()}
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session", State: StateRunning}}

	manager.queueACPMessageWithID(job, "message chunks, t", "message-1")
	manager.publishACPTool(job.eventView(), sessionevents.ACPToolCall{ID: "tool-1", Title: "Read file", Status: "completed"})
	manager.queueACPMessageWithID(job, "ool calls", "message-1")
	manager.withACPTranscriptBarrier(job.eventView(), nil)

	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != "acp_message" || events[2].Type != "acp_message" {
		t.Fatalf("text events = %#v", events)
	}
	firstRun := events[0].ACP.TextRunID
	secondRun := events[2].ACP.TextRunID
	if firstRun == "" || secondRun != firstRun {
		t.Fatalf("text run ids = %q, %q; events=%#v", firstRun, secondRun, events)
	}
	if events[1].Type != "acp_tool" || events[1].ACP.TextRunID != "" {
		t.Fatalf("tool event = %#v", events[1])
	}
	if events[0].ProjectionKey == "" || events[2].ProjectionKey != events[0].ProjectionKey || events[2].ProjectionOp != sessionevents.ProjectionAppend {
		t.Fatalf("persisted text projection = %#v, %#v", events[0], events[2])
	}
}

func TestACPTranscriptChunksWithoutMessageIDShareTurnRun(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session", State: StateRunning}}
	job.startTurn(CompletionInline, false, false)
	manager.jobsByID[session.ID] = job

	manager.queueACPMessage(job, "message chunks, ")
	manager.queueACPMessage(job, "stay together")
	manager.withACPTranscriptBarrier(job.eventView(), nil)

	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != "acp_message" || events[0].Content != "message chunks, stay together" || events[0].ACP.TextRunID == "" {
		t.Fatalf("text event = %#v", events[0])
	}
}

func TestACPTranscriptChunksWithoutMessageIDRunSurvivesTimerFlush(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session", State: StateRunning}}
	job.startTurn(CompletionInline, false, false)
	manager.jobsByID[session.ID] = job

	manager.queueACPThought(job, "Think")
	manager.flushACPTranscriptBuffer(manager.transcriptBuffers.get(session.ID, false))
	manager.queueACPThought(job, "ing")
	manager.withACPTranscriptBarrier(job.eventView(), nil)

	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != "acp_thought" || events[0].ACP.Thought != "Think" || events[0].ACP.TextRunID == "" {
		t.Fatalf("first thought event = %#v", events[0])
	}
	if events[1].Type != "acp_thought" || events[1].ACP.Thought != "ing" || events[1].ACP.TextRunID != events[0].ACP.TextRunID {
		t.Fatalf("second thought event = %#v", events[1])
	}
}

func TestACPTranscriptChunksWithoutMessageIDDoNotCrossTurns(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session", State: StateRunning}}
	manager.jobsByID[session.ID] = job

	job.startTurn(CompletionInline, false, false)
	manager.queueACPThought(job, "first")
	manager.withACPTranscriptBarrier(job.eventView(), nil)
	job.startTurn(CompletionInline, false, false)
	manager.queueACPThought(job, "second")
	manager.withACPTranscriptBarrier(job.eventView(), nil)

	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != "acp_thought" || events[0].ACP.TextRunID == "" {
		t.Fatalf("first thought event = %#v", events[0])
	}
	if events[1].Type != "acp_thought" || events[1].ACP.TextRunID == "" || events[1].ACP.TextRunID == events[0].ACP.TextRunID {
		t.Fatalf("second thought event = %#v", events[1])
	}
}

func TestACPTranscriptChunksWithoutMessageIDDoNotCrossToolBarrier(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session", State: StateRunning}}
	job.startTurn(CompletionInline, false, false)
	manager.jobsByID[session.ID] = job

	manager.queueACPMessage(job, "message chunks, t")
	manager.publishACPTool(job.eventView(), sessionevents.ACPToolCall{ID: "tool-1", Title: "Read file", Status: "completed"})
	manager.queueACPMessage(job, "ool calls")
	manager.withACPTranscriptBarrier(job.eventView(), nil)

	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != "acp_message" || events[0].Content != "message chunks, t" || events[0].ACP.TextRunID == "" {
		t.Fatalf("first text event = %#v", events[0])
	}
	if events[1].Type != "acp_tool" {
		t.Fatalf("tool event = %#v", events[1])
	}
	if events[2].Type != "acp_message" || events[2].Content != "ool calls" || events[2].ACP.TextRunID == "" || events[2].ACP.TextRunID == events[0].ACP.TextRunID {
		t.Fatalf("second text event = %#v", events[2])
	}
}

func TestACPTranscriptUpstreamMessageIDDefinesTextRun(t *testing.T) {
	first := textRunID("message-1")
	same := textRunID("message-1")
	second := textRunID("message-2")
	withoutMessageID := textRunID("")

	if first != "message:message-1" {
		t.Fatalf("first text run id = %q", first)
	}
	if same != first {
		t.Fatalf("same upstream message id produced %q, want %q", same, first)
	}
	if second == first {
		t.Fatalf("different upstream message id reused %q", second)
	}
	if withoutMessageID != "" {
		t.Fatalf("text run id without message id = %q, want empty", withoutMessageID)
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

func TestInactiveStatusContainsOnlySessionMetadata(t *testing.T) {
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
	if len(status.ToolCalls) != 0 || !status.LastToolAt.IsZero() {
		t.Fatalf("inactive status = %#v", status)
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

func TestStartTurnPublishesPlanClear(t *testing.T) {
	job := &jobState{Job: Job{
		ID:   "session-1",
		Plan: []sessionevents.PlanEntry{{Content: "Inspect sources", Status: "in_progress"}},
	}}
	job.startTurn(CompletionInline, false, false)

	raw, err := json.Marshal(EventFromJob(job.Snapshot()))
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	plan, ok := event["plan"].([]any)
	if !ok || len(plan) != 0 {
		t.Fatalf("turn-start event plan = %#v, want explicit empty clear", event["plan"])
	}

	job.startTurn(CompletionInline, false, false)
	raw, err = json.Marshal(EventFromJob(job.Snapshot()))
	if err != nil {
		t.Fatal(err)
	}
	event = map[string]any{}
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	if _, ok := event["plan"]; ok {
		t.Fatalf("plan-less turn start carries a plan signal: %s", raw)
	}
}

func TestEventFromJobExcludesDedicatedTranscriptFields(t *testing.T) {
	event := EventFromJob(Job{
		ID:          "session-1",
		Assistant:   "answer",
		Thought:     "reasoning",
		Plan:        []sessionevents.PlanEntry{{Content: "inspect"}},
		ToolCalls:   []sessionevents.ACPToolCall{{ID: "tool-1"}},
		Permissions: []sessionevents.ACPPermission{{ID: "permission-1"}},
	})

	if event.Assistant != "" || event.Thought != "" || event.ToolCalls != nil || event.Permissions != nil {
		t.Fatalf("event repeated dedicated transcript fields: %#v", event)
	}
	if len(event.Plan) != 1 || event.Plan[0].Content != "inspect" {
		t.Fatalf("event lost plan state: %#v", event.Plan)
	}
}
