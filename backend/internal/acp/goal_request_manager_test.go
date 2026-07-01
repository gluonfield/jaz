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
