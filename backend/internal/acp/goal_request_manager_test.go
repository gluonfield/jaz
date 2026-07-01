package acp_test

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestManagerSendRequestsCodexGoalWithoutLocalGoalSnapshot(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeCodexManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_EXPECT_PROMPT_CONTAINS": `create_goal`,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	session, err := manager.CreateSession(ctx, acp.SpawnRequest{
		ACPAgent:  acp.AgentCodex,
		Slug:      "codex-goal",
		Directory: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.Capabilities == nil || !session.RuntimeRef.Capabilities.NativeGoal {
		t.Fatalf("stored runtime ref = %#v, want native goal capability", session.RuntimeRef)
	}
	if _, err := manager.Send(ctx, acp.SendRequest{
		Session:       session.ID,
		Message:       "say hello",
		Completion:    acp.CompletionInline,
		GoalRequested: true,
	}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: session.ID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), session.ID) }()
	if job.State != acp.StateIdle {
		t.Fatalf("job = %#v", job)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RuntimeRef == nil || loaded.RuntimeRef.Capabilities == nil || !loaded.RuntimeRef.Capabilities.NativeGoal {
		t.Fatalf("loaded runtime ref = %#v, want native goal capability", loaded.RuntimeRef)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var goalEvent *sessionevents.GoalEvent
	for i := range events {
		if events[i].Type == sessionevents.TypeGoalUpdate {
			goalEvent = events[i].Goal
		}
	}
	if goalEvent != nil {
		t.Fatalf("stored events = %#v, want no local goal update before provider report", events)
	}
	if loaded.Goal != nil {
		t.Fatalf("session goal = %#v, want nil before provider report", loaded.Goal)
	}
}

func TestManagerPersistsProviderGoalObjectiveNotUserPrompt(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const providerObjective = "provider refined goal objective"
	const userPrompt = "continue iterating and commit periodically"
	manager := newFakeCodexManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_EXPECT_PROMPT_CONTAINS": `create_goal`,
		"JAZ_FAKE_ACP_GOAL_OBJECTIVE":         providerObjective,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	session, err := manager.CreateSession(ctx, acp.SpawnRequest{
		ACPAgent:  acp.AgentCodex,
		Slug:      "codex-goal",
		Directory: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Send(ctx, acp.SendRequest{
		Session:       session.ID,
		Message:       userPrompt,
		Completion:    acp.CompletionInline,
		GoalRequested: true,
	}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: session.ID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), session.ID) }()
	if job.State != acp.StateIdle {
		t.Fatalf("job = %#v", job)
	}

	loaded := session
	deadline := time.Now().Add(2 * time.Second)
	for {
		next, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if next.Goal != nil {
			loaded = next
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session goal was not persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if loaded.Goal.Objective != providerObjective {
		t.Fatalf("goal objective = %q, want provider objective %q", loaded.Goal.Objective, providerObjective)
	}
	if loaded.Goal.Objective == userPrompt {
		t.Fatalf("goal objective used the user prompt: %#v", loaded.Goal)
	}
	if loaded.Goal.Provider != acp.AgentCodex ||
		loaded.Goal.ProviderGoalID != "fake-goal-1" ||
		loaded.Goal.TokensUsed != 42 ||
		loaded.Goal.TokenBudget == nil ||
		*loaded.Goal.TokenBudget != 1000 ||
		loaded.Goal.RemainingTokens == nil ||
		*loaded.Goal.RemainingTokens != 958 {
		t.Fatalf("goal state = %#v", loaded.Goal)
	}
}
