package acp

import (
	"github.com/wins/jaz/backend/internal/sessionevents"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"testing"
	"time"
)

func TestActivityInvalidationIsSafeAndThrottledWithinPhase(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	events := manager.Events.Subscribe(t.Context(), "bot", sessionevents.TypeSession)
	now := time.Now().UTC()
	job := &jobState{Job: Job{ID: "bot"}}
	assertEvent := func() {
		t.Helper()
		select {
		case event := <-events:
			if event.ACP != nil || event.Content != "" || event.Type != sessionevents.TypeSession {
				t.Fatal("activity invalidation contained a payload")
			}
		case <-time.After(time.Second):
			t.Fatal("missing activity invalidation")
		}
	}
	job.recordActivityLocked("reasoning", "", now)
	manager.publishActivityProgress(job)
	assertEvent()
	job.recordActivityLocked("reasoning", "", now.Add(time.Millisecond))
	manager.publishActivityProgress(job)
	select {
	case <-events:
		t.Fatal("token chunk bypassed activity throttle")
	default:
	}
	job.recordActivityLocked("responding", "", now.Add(2*time.Millisecond))
	manager.publishActivityProgress(job)
	assertEvent()
	job.activityPublishedAt = now.Add(-4 * time.Second)
	job.recordActivityLocked("responding", "", now.Add(3*time.Millisecond))
	manager.publishActivityProgress(job)
	assertEvent()
}

func TestSafeToolProgramNeverIncludesShellArguments(t *testing.T) {
	for _, tc := range []struct {
		command any
		want    string
	}{
		{"ssh user:SECRET@host", "ssh"}, {[]any{"curl", "-u", "user:SECRET"}, "curl"}, {"python3 secret.py", "python3"},
		{"TOKEN=SECRET curl site", ""}, {"/secret/bin/curl token", ""}, {"bash -c 'curl SECRET'", ""}, {"sudo curl SECRET", ""}, {"curl; echo SECRET", ""},
	} {
		if got := safeToolProgram(map[string]any{"command": tc.command}); got != tc.want {
			t.Fatalf("safe program=%q want=%q", got, tc.want)
		}
	}
}

func TestThinkingRequiresAnActualReasoningEvent(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{Job: Job{ID: "bot", State: StateRunning}, turn: &activeTurn{}}
	manager.jobsByID["bot"] = job
	if manager.Thinking("bot") {
		t.Fatal("absence of tools was mistaken for reasoning")
	}
	job.recordActivityLocked("reasoning", "", time.Now().UTC())
	if !manager.Thinking("bot") {
		t.Fatal("reported reasoning was lost")
	}
	job.recordActivityLocked("responding", "", time.Now().UTC())
	if manager.Thinking("bot") {
		t.Fatal("response generation was mistaken for reasoning")
	}
}
