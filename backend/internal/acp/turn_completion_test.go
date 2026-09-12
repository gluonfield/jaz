package acp

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
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

type blockedTurnEvents struct {
	Store
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockedTurnEvents) AppendSessionEvents(id string, events ...sessionevents.Event) error {
	s.once.Do(func() {
		close(s.entered)
		<-s.release
	})
	return s.Store.AppendSessionEvents(id, events...)
}

func TestPromptCompletionHasOneOwner(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "failed"}[failFirst], func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			gate := &blockedTurnEvents{Store: store, entered: make(chan struct{}), release: make(chan struct{})}
			defer close(gate.release)
			session, err := store.CreateSession(storage.CreateSession{Slug: "completion", Runtime: storage.RuntimeACP})
			if err != nil {
				t.Fatal(err)
			}
			manager := NewManager(gate, Config{}, log.New(io.Discard))
			job := newIdleJob(session, AgentGrok, "native-session", "", ModeState{})
			manager.addJob(job, nil)
			done := job.startTurn(CompletionInline, false, false)
			job.Assistant = "Native answer"
			settle := func(fail bool) {
				if fail {
					manager.failPromptCall(done, job, errors.New("connection lost"))
				} else {
					manager.completePromptCall(done, job, StopReasonEndTurn)
				}
			}
			firstDone := make(chan struct{})
			go func() {
				settle(failFirst)
				close(firstDone)
			}()
			t.Cleanup(func() { <-firstDone })
			<-gate.entered
			rivalDone := make(chan struct{})
			go func() {
				settle(!failFirst)
				close(rivalDone)
			}()
			t.Cleanup(func() { <-rivalDone })
			select {
			case <-rivalDone:
			case <-time.After(time.Second):
				t.Fatal("another completion entered while the first was being persisted")
			}
			want := StateIdle
			if failFirst {
				want = StateFailed
			}
			if got := job.Snapshot().State; got != want {
				t.Fatalf("rival changed the terminal state to %s, want %s", got, want)
			}
		})
	}
}

type blockedTurnError struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *blockedTurnError) Error() string {
	e.once.Do(func() {
		close(e.entered)
		<-e.release
	})
	return "connection lost"
}

func TestSteerCannotJoinFailingTurn(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "failing", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, log.New(io.Discard))
	job := newIdleJob(session, AgentGrok, "native-session", "", ModeState{})
	job.steerMethod = steerGrokInterject
	manager.addJob(job, nil)
	done := job.startTurn(CompletionInline, false, false)
	gate := &blockedTurnError{entered: make(chan struct{}), release: make(chan struct{})}
	failed := make(chan struct{})
	go func() {
		manager.failPromptCall(done, job, gate)
		close(failed)
	}()
	t.Cleanup(func() {
		close(gate.release)
		<-failed
	})
	<-gate.entered
	if _, err := manager.reserveSteer(job, SteerRequest{Message: "Late correction"}, nil); !errors.Is(err, errTurnEnded) {
		t.Errorf("steering joined a failing turn: %v", err)
	}
	if conflict := job.sendConflict(); conflict == nil || !conflict.finishing {
		t.Errorf("new input would steer instead of waiting for failure to finish: %+v", conflict)
	}
	messages, err := store.LoadMessageRecords(session.ID)
	if err != nil || len(messages) != 0 {
		t.Fatalf("correction persisted against failed work: %+v, %v", messages, err)
	}
}
