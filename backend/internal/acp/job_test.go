package acp

import (
	"context"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestRequestShutdownOnlyMarksActiveTurns(t *testing.T) {
	job := &jobState{
		Job:  Job{State: StateIdle},
		turn: &activeTurn{done: make(chan struct{})},
	}

	if job.requestShutdown() {
		t.Fatal("idle cleanup turn marked as shutdown")
	}
	if reason, ok := job.cancelReason(); ok {
		t.Fatalf("cancel reason = %q, want none", reason)
	}

	job.State = StateRunning
	if !job.requestShutdown() {
		t.Fatal("running turn was not marked as shutdown")
	}
	if reason, ok := job.cancelReason(); !ok || reason != StopReasonServerShutdown {
		t.Fatalf("cancel reason = %q/%t, want server_shutdown", reason, ok)
	}
}

func TestCompletedTurnPayloadIsReleasedAndRestoredOnDemand(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "released-result", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	acpEvent := func(state string) *sessionevents.ACPEvent {
		return &sessionevents.ACPEvent{ID: session.ID, State: state}
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "old", ACP: acpEvent(StateRunning)},
		sessionevents.Event{Type: "acp", ACP: acpEvent(StateIdle)},
		sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "Falling back from WebSockets to HTTPS transport. disconnected", ACP: &sessionevents.ACPEvent{ID: session.ID, State: StateRunning, TextRunID: "message:codex:warning:turn:1"}},
		sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "new answer", ACP: acpEvent(StateRunning)},
		sessionevents.Event{Type: sessionevents.TypeACPThought, ACP: &sessionevents.ACPEvent{ID: session.ID, State: StateRunning, Thought: "new thought"}},
		sessionevents.Event{Type: "acp_tool", ACP: &sessionevents.ACPEvent{ID: session.ID, State: StateRunning, ToolCalls: []sessionevents.ACPToolCall{{ID: "tool-2", Title: "Read", Status: "completed"}}}},
		sessionevents.Event{Type: "acp", ACP: acpEvent(StateIdle)},
	); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(store, Config{}, nil)
	job := newIdleJob(session, AgentCodex, "acp-session", "", ModeState{})
	job.Assistant = "new answer"
	job.Thought = "new thought"
	job.ToolCalls = []sessionevents.ACPToolCall{{ID: "tool-2", RawOutput: []byte(`"large"`)}}
	job.turnResultDiscarded = false
	manager.addJob(job, nil)

	release := manager.RetainStream(session.ID)
	manager.discardTurnResultWhenReleased(job)
	if job.resultDiscarded() {
		t.Fatal("stream reader did not retain completed result")
	}
	release()
	if !job.resultDiscarded() || job.Snapshot().Assistant != "" || len(job.Snapshot().ToolCalls) != 0 {
		t.Fatalf("completed payload was retained: %#v", job.Snapshot())
	}

	restored, err := manager.Wait(t.Context(), WaitRequest{Session: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Assistant != "new answer" || restored.Thought != "new thought" {
		t.Fatalf("restored text = %q/%q", restored.Assistant, restored.Thought)
	}
	if len(restored.ToolCalls) != 1 || restored.ToolCalls[0].ID != "tool-2" || restored.ToolCalls[0].Title != "Read" {
		t.Fatalf("restored tools = %#v", restored.ToolCalls)
	}
}

func TestHydrationJobsReadsOnlyRequestedLightweightState(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	wantedSession := storage.Session{ID: "wanted", Slug: "wanted", Runtime: storage.RuntimeACP}
	wanted := newIdleJob(wantedSession, AgentCodex, "acp-wanted", "", ModeState{})
	wanted.Assistant = "large answer"
	wanted.Thought = "large thought"
	wanted.ToolCalls = []sessionevents.ACPToolCall{{ID: "tool", RawOutput: []byte("large output")}}
	wanted.Plan = []sessionevents.PlanEntry{{Content: "Keep metadata"}}
	wanted.Permissions = []sessionevents.ACPPermission{{ID: "permission"}}
	manager.addJob(wanted, nil)
	other := newIdleJob(storage.Session{ID: "other", Slug: "other", Runtime: storage.RuntimeACP}, AgentCodex, "acp-other", "", ModeState{})
	manager.addJob(other, nil)

	views := manager.HydrationJobs([]string{wanted.ID, "missing"})
	view, ok := views[wanted.ID]
	if !ok || len(views) != 1 {
		t.Fatalf("hydration views = %#v", views)
	}
	job := view.Job()
	if job.Assistant != "" || job.Thought != "" || len(job.ToolCalls) != 0 {
		t.Fatalf("hydration copied turn payload: %#v", job)
	}
	if len(view.Plan) != 1 || len(view.Permissions) != 1 {
		t.Fatalf("hydration lost live metadata: %#v", view)
	}
}

func TestLateLocalCancelObservesShutdownRequest(t *testing.T) {
	done := make(chan struct{})
	job := &jobState{
		Job:  Job{State: StateRunning},
		turn: &activeTurn{done: done},
	}
	if !job.requestShutdown() {
		t.Fatal("running turn was not marked as shutdown")
	}
	ctx, cancel := context.WithCancel(context.Background())
	if !job.setTurnCancel(done, cancel) {
		t.Fatal("active turn rejected its cancel function")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("late cancel function did not observe the shutdown request")
	}
	if job.setTurnCancel(make(chan struct{}), func() {}) {
		t.Fatal("stale turn installed a cancel function")
	}
}

func TestJobStreamViewContainsOnlyStreamFields(t *testing.T) {
	job := &jobState{Job: Job{
		Assistant: "answer",
		Thought:   "reasoning",
		ToolCalls: []sessionevents.ACPToolCall{{
			ID:        "tool-1",
			Title:     "Read file",
			Content:   []sessionevents.ACPToolContent{{Type: "text", Text: "large output"}},
			RawInput:  map[string]any{"path": "/large"},
			RawOutput: []byte(`"large output"`),
		}},
		Permissions: []sessionevents.ACPPermission{{ID: "permission-1"}},
	}}

	snapshot := job.streamView()
	if snapshot.Assistant != "answer" || snapshot.Thought != "reasoning" {
		t.Fatalf("stream text = %q/%q", snapshot.Assistant, snapshot.Thought)
	}
	if len(snapshot.Tools) != 1 || snapshot.Tools[0].ID != "tool-1" || snapshot.Tools[0].Title != "Read file" {
		t.Fatalf("stream tool identity = %#v", snapshot.Tools)
	}
}
