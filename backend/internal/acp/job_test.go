package acp

import (
	"context"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
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

func TestJobStreamSnapshotOmitsHeavyToolPayload(t *testing.T) {
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

	snapshot := job.streamSnapshot()
	if snapshot.Assistant != "answer" || snapshot.Thought != "reasoning" {
		t.Fatalf("stream text = %q/%q", snapshot.Assistant, snapshot.Thought)
	}
	if len(snapshot.ToolCalls) != 1 || snapshot.ToolCalls[0].ID != "tool-1" || snapshot.ToolCalls[0].Title != "Read file" {
		t.Fatalf("stream tool identity = %#v", snapshot.ToolCalls)
	}
	if snapshot.ToolCalls[0].Content != nil || snapshot.ToolCalls[0].RawInput != nil || snapshot.ToolCalls[0].RawOutput != nil || snapshot.Permissions != nil {
		t.Fatalf("stream snapshot retained heavy payload: %#v", snapshot)
	}
}
