package acp

import (
	"context"
	"strings"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestNativeGoalUpdatesAreIgnored(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	session.Goal = activeGoalState("jaz-owned goal", 1200)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, req := range []jsonrpc.Request{
		{
			Method: acpMethodCodexGoalUpdated,
			Params: mustJSON(t, map[string]any{
				"threadId": "acp-session",
				"goal": map[string]any{
					"objective":  "provider goal",
					"status":     "active",
					"tokensUsed": 1_500_000,
				},
			}),
		},
		{
			Method: acpschema.ClientMethodSessionUpdate,
			Params: mustJSON(t, map[string]any{
				"sessionId": "acp-session",
				"update": map[string]any{
					"sessionUpdate": acpSessionUpdateGoal,
					"goal": map[string]any{
						"objective":   "provider goal",
						"status":      "active",
						"tokens_used": 1_500_000,
					},
				},
			}),
		},
	} {
		raw, rpcErr := manager.handleJSONRPC(ctx, req)
		if rpcErr != nil {
			t.Fatal(rpcErr)
		}
		if string(raw) != "{}" {
			t.Fatalf("response = %s", raw)
		}
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal == nil || loaded.Goal.Objective != "jaz-owned goal" || loaded.Goal.TokensUsed != 1200 {
		t.Fatalf("session goal = %#v", loaded.Goal)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("stored events = %#v, want none", stored)
	}
}

func TestNativeGoalClearsAreIgnored(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-goal-clear", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	session.Goal = activeGoalState("jaz-owned goal", 1200)
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, req := range []jsonrpc.Request{
		{
			Method: acpMethodCodexGoalCleared,
			Params: mustJSON(t, map[string]any{"threadId": "acp-session"}),
		},
		{
			Method: acpschema.ClientMethodSessionUpdate,
			Params: mustJSON(t, map[string]any{
				"sessionId": "acp-session",
				"update":    map[string]any{"sessionUpdate": acpSessionUpdateGoalClear},
			}),
		},
	} {
		raw, rpcErr := manager.handleJSONRPC(ctx, req)
		if rpcErr != nil {
			t.Fatal(rpcErr)
		}
		if string(raw) != "{}" {
			t.Fatalf("response = %s", raw)
		}
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal == nil || loaded.Goal.Objective != "jaz-owned goal" || loaded.Goal.TokensUsed != 1200 {
		t.Fatalf("session goal = %#v", loaded.Goal)
	}
}

func TestGoalPromptUsesJazToolsNotProviderMeta(t *testing.T) {
	prompt := goalPromptMessage("do the work", true)
	if !strings.Contains(prompt, "create_goal") || !strings.Contains(prompt, "do the work") {
		t.Fatalf("goal prompt message = %q", prompt)
	}
	if got := goalPromptMessage("do the work", false); got != "do the work" {
		t.Fatalf("unrequested goal prompt message = %q", got)
	}
}

func TestInitNativeGoalSupportedReadsCodexInitializeMeta(t *testing.T) {
	raw := mustJSON(t, map[string]any{
		"agentCapabilities": map[string]any{
			"_meta": map[string]any{
				"codex": map[string]any{"nativeGoal": true},
			},
		},
	})
	if !initNativeGoalSupported(raw) {
		t.Fatalf("native goal support was not detected")
	}
	if initNativeGoalSupported(mustJSON(t, map[string]any{"agentCapabilities": map[string]any{}})) {
		t.Fatalf("native goal support detected without meta")
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
