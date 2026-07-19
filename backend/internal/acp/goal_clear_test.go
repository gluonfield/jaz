package acp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestGoalPromptUsesJazTools(t *testing.T) {
	prompt := goalPromptMessage("do the work", true)
	for _, required := range []string{"create_goal", "do the work", "user explicitly provided", "Never estimate or invent one", "does not interrupt a turn already in progress"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("goal prompt message missing %q: %q", required, prompt)
		}
	}
	if got := goalPromptMessage("do the work", false); got != "do the work" {
		t.Fatalf("unrequested goal prompt message = %q", got)
	}
}

func TestCancelClearsPersistedGoal(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-goal-cancel", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	session.Goal = activeGoalState("Stop should clear this", 10)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	if _, err := manager.Cancel(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	_ = receiveGoalClear(t, ctx, sub)
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal != nil {
		t.Fatalf("session goal after cancel = %#v", loaded.Goal)
	}
}

func TestCancelStoredClearsPersistedGoal(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-goal-stored-cancel", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	session.Goal = activeGoalState("Stop should clear after restart", 10)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	if _, err := manager.Cancel(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	_ = receiveGoalClear(t, ctx, sub)
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal != nil {
		t.Fatalf("session goal after stored cancel = %#v", loaded.Goal)
	}
}

func activeGoalState(objective string, tokensUsed int64) *goal.State {
	return &goal.State{
		Identity: goal.Identity{
			Objective: objective,
			Status:    goal.StatusActive,
		},
		Budget: goal.Budget{TokensUsed: tokensUsed},
	}
}

func receiveGoalClear(t *testing.T, ctx context.Context, sub <-chan sessionevents.Event) sessionevents.Event {
	t.Helper()
	for {
		select {
		case event := <-sub:
			if event.Type == sessionevents.TypeGoalClear {
				return event
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
