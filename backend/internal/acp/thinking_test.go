package acp

import (
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

// A turn thinks until it calls its first tool; a thread with no turn does not.
func TestThinkingLastsUntilTheFirstToolCall(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{Job: Job{ID: "bot", State: StateIdle}}
	manager.jobsByID["bot"] = job
	if manager.Thinking("bot") || manager.Thinking("nobody") {
		t.Fatal("a thread without a running turn is thinking")
	}
	job.State = StateRunning
	if !manager.Thinking("bot") {
		t.Fatal("a turn that has called no tool is not thinking")
	}
	job.ToolCalls = []sessionevents.ACPToolCall{{ID: "t", Title: "Read the inbox"}}
	if manager.Thinking("bot") {
		t.Fatal("a turn that called a tool is still thinking")
	}
}
