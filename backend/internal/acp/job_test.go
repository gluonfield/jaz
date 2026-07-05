package acp

import "testing"

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
