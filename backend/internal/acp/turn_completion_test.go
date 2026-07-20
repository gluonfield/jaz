package acp

import (
	"context"
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestEndTurnRequiresVisibleResult(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation string
		prepare   func(*jobState)
		wantState string
	}{
		{name: "empty", wantState: StateFailed},
		{name: "thought only", prepare: func(job *jobState) { job.Thought = "unfinished reasoning" }, wantState: StateFailed},
		{name: "assistant", prepare: func(job *jobState) { job.Assistant = "done" }, wantState: StateIdle},
		{name: "tool", prepare: func(job *jobState) { job.ToolCalls = []sessionevents.ACPToolCall{{ID: "tool"}} }, wantState: StateIdle},
		{name: "plan", prepare: func(job *jobState) { job.Plan = []sessionevents.PlanEntry{{Content: "done"}} }, wantState: StateIdle},
		{name: "plan proposal", prepare: func(job *jobState) { job.turn.planDocument = "proposed plan" }, wantState: StateIdle},
		{name: "compaction", operation: ActiveOperationCompact, wantState: StateIdle},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			session, err := store.CreateSession(storage.CreateSession{Slug: "turn", Runtime: storage.RuntimeACP})
			if err != nil {
				t.Fatal(err)
			}
			manager := NewManager(store, Config{}, log.New(io.Discard))
			job := newIdleJob(session, AgentKimi, "acp-session", "", ModeState{})
			done := job.startTurnWithOperation(CompletionInline, false, false, test.operation)
			if test.prepare != nil {
				test.prepare(job)
			}
			finished := make(chan Job, 1)
			manager.TurnFinished = func(_ context.Context, result Job) { finished <- result }

			manager.completePromptCall(done, job, StopReasonEndTurn)
			result := <-finished
			if result.State != test.wantState {
				t.Fatalf("state = %q, want %q; error=%q", result.State, test.wantState, result.Error)
			}
			if test.wantState == StateFailed && result.Error == "" {
				t.Fatal("failed empty turn has no visible error")
			}
			if test.wantState == StateIdle && result.Error != "" {
				t.Fatalf("successful turn error = %q", result.Error)
			}
		})
	}
}
