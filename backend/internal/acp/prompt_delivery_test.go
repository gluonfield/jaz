package acp

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

// An agent that queues prompts takes one in when it answers the prompt before
// it, or at once when it works on nothing else; until then the turn keeps it.
func TestTurnKeepsEachSteerUntilTheAgentTakesItIn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "kept-steers", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(session.ID, storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := newIdleJob(session, "fake", "runtime-session", "", ModeState{})
	job.steerMethod = steerPromptQueueing
	job.startTurn(CompletionInline, false, false)
	job.turn.promptObserved = true
	send := func(id string) {
		t.Helper()
		if err := manager.promptSending(job, id); err != nil {
			t.Fatal(err)
		}
		manager.promptSent(job)
	}
	steer := func(text string) {
		t.Helper()
		if _, err := manager.reserveSteer(job, SteerRequest{Session: session.ID, Message: text}, nil, text); err != nil {
			t.Fatal(err)
		}
	}
	kept := func(want ...string) {
		t.Helper()
		stored, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, steer := range stored.Turn.Steers {
			got = append(got, steer.Message)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("kept steers = %q, want %q", got, want)
		}
	}

	send("opening")
	steer("a")
	send("a")
	steer("b")
	kept("a", "b")
	manager.promptAnswered(job, "opening")
	kept("b")
	send("b")
	manager.promptAnswered(job, "a")
	kept()
	manager.promptAnswered(job, "b")
	steer("c")
	kept("c")
	send("c")
	kept()

	// The agent's own run reports no take-in, so a steer into it is not kept.
	if err := store.StartSessionTurn(session.ID, storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	job.mu.Lock()
	run := job.beginTurnLocked(CompletionInline, storage.Turn{})
	run.agentRun, run.promptObserved, run.unsent = true, true, 0
	run.closeFirstPromptSent()
	job.mu.Unlock()
	steer("d")
	kept()
}
