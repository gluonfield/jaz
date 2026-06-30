package acp

import (
	"context"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestGoalSessionUpdatePublishesAndPersistsGoal(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	createdAt := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC).Unix()
	updatedAt := time.Date(2026, 6, 25, 10, 5, 0, 0, time.UTC).Unix()

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": acpSessionUpdateGoal,
				"goal": map[string]any{
					"threadId":        "thread-1",
					"providerGoalId":  "goal-1",
					"objective":       "Ship native ACP goal support",
					"status":          "active",
					"budgetSource":    "goal",
					"tokenBudget":     1000,
					"tokensUsed":      250,
					"timeUsedSeconds": 45,
					"evaluatedTurns":  3,
					"progressMessage": "Tests are running",
					"createdAt":       createdAt,
					"updatedAt":       updatedAt,
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	event := receiveGoalUpdate(t, ctx, sub)
	if event.Goal.ThreadID != "thread-1" ||
		event.Goal.Source != goal.SourceProvider ||
		event.Goal.Provider != AgentCodex ||
		event.Goal.ProviderGoalID != "goal-1" ||
		event.Goal.Objective != "Ship native ACP goal support" ||
		event.Goal.Status != "active" ||
		event.Goal.BudgetSource != "goal" ||
		event.Goal.TokenBudget == nil ||
		*event.Goal.TokenBudget != 1000 ||
		event.Goal.TokensUsed != 250 ||
		event.Goal.RemainingTokens == nil ||
		*event.Goal.RemainingTokens != 750 ||
		event.Goal.TimeUsedSeconds != 45 ||
		event.Goal.EvaluatedTurns != 3 ||
		event.Goal.ProgressMessage != "Tests are running" {
		t.Fatalf("goal event = %#v", event.Goal)
	}
	if got := manager.jobsByID[session.ID].Assistant; got != "" {
		t.Fatalf("main assistant was mutated: %q", got)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Type != sessionevents.TypeGoalUpdate || stored[0].Goal == nil {
		t.Fatalf("stored events = %#v", stored)
	}
	if stored[0].Goal.ThreadID != "thread-1" || stored[0].Goal.Source != goal.SourceProvider || stored[0].Goal.ProviderGoalID != "goal-1" || stored[0].Goal.TokensUsed != 250 {
		t.Fatalf("stored goal event = %#v", stored[0])
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal == nil || loaded.Goal.Source != goal.SourceProvider ||
		loaded.Goal.Status != goal.StatusActive || loaded.Goal.Objective != "Ship native ACP goal support" ||
		loaded.Goal.RemainingTokens == nil || *loaded.Goal.RemainingTokens != 750 ||
		loaded.Goal.ProviderGoalID != "goal-1" ||
		loaded.Goal.ProgressMessage != "Tests are running" {
		t.Fatalf("session goal = %#v", loaded.Goal)
	}
}

func TestGoalPromptMetaMarksGoalRequested(t *testing.T) {
	meta := goalPromptMeta(true, "  Finish the goal  ")
	codex, ok := meta[codexMetaKey].(map[string]any)
	if !ok {
		t.Fatalf("goal prompt meta = %#v", meta)
	}
	goal, ok := codex["goal"].(map[string]any)
	if !ok || goal["requested"] != true || goal["objective"] != "Finish the goal" {
		t.Fatalf("goal prompt meta = %#v", meta)
	}
	if got := goalPromptMeta(false, "ignored"); got != nil {
		t.Fatalf("unrequested goal prompt meta = %#v", got)
	}
}

func TestDecodeGoalUpdateRejectsPartialSnapshot(t *testing.T) {
	_, ok := decodeGoalUpdate(mustJSON(t, map[string]any{
		"sessionId": "acp-session",
		"update": map[string]any{
			"sessionUpdate": acpSessionUpdateGoal,
			"goal": map[string]any{
				"status":          "active",
				"progressMessage": "still working",
			},
		},
	}))
	if ok {
		t.Fatalf("partial goal update decoded as complete")
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

func TestGoalExtensionNotificationPublishesAndPersistsGoal(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-goal-extension", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpMethodGoalUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"goal": map[string]any{
				"thread_id":          "thread-2",
				"goal_id":            "goal-2",
				"objective":          "Finish via Codex native goal",
				"status":             "budget_limited",
				"token_budget":       100,
				"tokens_used":        120,
				"time_used_seconds":  9,
				"blocked_reason":     "token budget reached",
				"completion_review":  "not_achieved",
				"active_subagent_id": "worker-1",
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	event := receiveGoalUpdate(t, ctx, sub)
	if event.Goal.ThreadID != "thread-2" ||
		event.Goal.Source != goal.SourceProvider ||
		event.Goal.ProviderGoalID != "goal-2" ||
		event.Goal.Status != "budgetLimited" ||
		event.Goal.RemainingTokens == nil ||
		*event.Goal.RemainingTokens != 0 ||
		event.Goal.BlockedReason != "token budget reached" ||
		event.Goal.CompletionReview != "not_achieved" ||
		event.Goal.ActiveSubagentID != "worker-1" {
		t.Fatalf("goal event = %#v", event.Goal)
	}

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Type != sessionevents.TypeGoalUpdate || stored[0].Goal == nil {
		t.Fatalf("stored events = %#v", stored)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal == nil || loaded.Goal.Source != goal.SourceProvider ||
		loaded.Goal.Status != goal.StatusBudgetLimited ||
		loaded.Goal.RemainingTokens == nil || *loaded.Goal.RemainingTokens != 0 ||
		loaded.Goal.ProviderGoalID != "goal-2" ||
		loaded.Goal.BlockedReason != "token budget reached" {
		t.Fatalf("session goal = %#v", loaded.Goal)
	}
}

func TestCodexNativeGoalNotificationPublishesAndPersistsGoal(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-native-goal-notification", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "codex-thread"}}
	manager.jobsByACP["codex-thread"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpMethodCodexGoalUpdated,
		Params: mustJSON(t, map[string]any{
			"threadId": "codex-thread",
			"goal": map[string]any{
				"threadId":        "codex-thread",
				"objective":       "Activate native Codex goal",
				"status":          "active",
				"tokenBudget":     1000,
				"tokensUsed":      10,
				"timeUsedSeconds": 2,
				"createdAt":       int64(1782736800),
				"updatedAt":       int64(1782736802),
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	event := receiveGoalUpdate(t, ctx, sub)
	if event.Goal.ThreadID != "codex-thread" ||
		event.Goal.Objective != "Activate native Codex goal" ||
		event.Goal.Status != goal.StatusActive ||
		event.Goal.TokenBudget == nil ||
		*event.Goal.TokenBudget != 1000 ||
		event.Goal.TokensUsed != 10 {
		t.Fatalf("goal event = %#v", event.Goal)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal == nil || loaded.Goal.Objective != "Activate native Codex goal" || loaded.Goal.Status != goal.StatusActive {
		t.Fatalf("session goal = %#v", loaded.Goal)
	}
}

func TestGoalSessionUpdateClearPublishesAndClearsPersistedGoal(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-goal-clear", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": acpSessionUpdateGoal,
				"goal": map[string]any{
					"objective": "Ship native ACP goal support",
					"status":    "active",
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	_ = receiveGoalUpdate(t, ctx, sub)
	if loaded, err := store.LoadSession(session.ID); err != nil || loaded.Goal == nil {
		t.Fatalf("stored goal before clear = %#v, err = %v", loaded.Goal, err)
	}

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": acpSessionUpdateGoalClear,
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}
	event := receiveGoalClear(t, ctx, sub)
	if event.Goal != nil {
		t.Fatalf("clear event carried goal = %#v", event.Goal)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal != nil {
		t.Fatalf("session goal after clear = %#v", loaded.Goal)
	}
}

func TestGoalClearNotificationPublishesAndClearsPersistedGoal(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-goal-clear-notification", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	manager.publishGoalUpdate(manager.jobsByID[session.ID], sessionevents.GoalEvent{
		Identity: goal.Identity{
			Objective: "Finish via Codex native goal",
			Status:    goal.StatusActive,
		},
	})
	_ = receiveGoalUpdate(t, ctx, sub)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpMethodCodexGoalCleared,
		Params: mustJSON(t, map[string]any{"threadId": "acp-session"}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}
	_ = receiveGoalClear(t, ctx, sub)
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal != nil {
		t.Fatalf("session goal after clear = %#v", loaded.Goal)
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
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	manager.publishGoalUpdate(manager.jobsByID[session.ID], sessionevents.GoalEvent{
		Identity: goal.Identity{
			Objective: "Stop should clear this",
			Status:    goal.StatusActive,
		},
	})
	_ = receiveGoalUpdate(t, ctx, sub)

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
	session.Goal = &goal.State{
		Identity: goal.Identity{
			Source:    goal.SourceProvider,
			Objective: "Stop should clear after restart",
			Status:    goal.StatusActive,
		},
	}
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

func receiveGoalUpdate(t *testing.T, ctx context.Context, sub <-chan sessionevents.Event) sessionevents.Event {
	t.Helper()
	for {
		select {
		case event := <-sub:
			if event.Type == sessionevents.TypeGoalUpdate && event.Goal != nil {
				return event
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
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
