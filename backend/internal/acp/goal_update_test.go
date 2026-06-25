package acp

import (
	"context"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
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
					"objective":       "Ship native ACP goal support",
					"status":          "active",
					"tokenBudget":     1000,
					"tokensUsed":      250,
					"timeUsedSeconds": 45,
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

	select {
	case event := <-sub:
		if event.Type != sessionevents.TypeGoalUpdate || event.Goal == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		if event.Goal.ThreadID != "thread-1" ||
			event.Goal.Objective != "Ship native ACP goal support" ||
			event.Goal.Status != "active" ||
			event.Goal.TokenBudget == nil ||
			*event.Goal.TokenBudget != 1000 ||
			event.Goal.TokensUsed != 250 ||
			event.Goal.RemainingTokens == nil ||
			*event.Goal.RemainingTokens != 750 ||
			event.Goal.TimeUsedSeconds != 45 {
			t.Fatalf("goal event = %#v", event.Goal)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
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
	if stored[0].Goal.ThreadID != "thread-1" || stored[0].Goal.TokensUsed != 250 {
		t.Fatalf("stored goal event = %#v", stored[0])
	}
}

func TestGoalPromptMetaMarksGoalRequested(t *testing.T) {
	meta := goalPromptMeta(true)
	jaz, ok := meta[jazMetaKey].(map[string]any)
	if !ok || jaz["goalRequested"] != true {
		t.Fatalf("goal prompt meta = %#v", meta)
	}
	if got := goalPromptMeta(false); got != nil {
		t.Fatalf("unrequested goal prompt meta = %#v", got)
	}
}

func TestNativeGoalSupportedReadsJazInitializeMeta(t *testing.T) {
	raw := mustJSON(t, map[string]any{
		"agentCapabilities": map[string]any{
			"_meta": map[string]any{
				"jaz": map[string]any{"nativeGoal": true},
			},
		},
	})
	if !nativeGoalSupported(raw) {
		t.Fatalf("native goal support was not detected")
	}
	if nativeGoalSupported(mustJSON(t, map[string]any{"agentCapabilities": map[string]any{}})) {
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
				"threadId":        "thread-2",
				"objective":       "Finish via Codex native goal",
				"status":          "budgetLimited",
				"tokenBudget":     100,
				"tokensUsed":      120,
				"timeUsedSeconds": 9,
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if string(raw) != "{}" {
		t.Fatalf("response = %s", raw)
	}

	select {
	case event := <-sub:
		if event.Type != sessionevents.TypeGoalUpdate || event.Goal == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		if event.Goal.ThreadID != "thread-2" ||
			event.Goal.Status != "budgetLimited" ||
			event.Goal.RemainingTokens == nil ||
			*event.Goal.RemainingTokens != 0 {
			t.Fatalf("goal event = %#v", event.Goal)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Type != sessionevents.TypeGoalUpdate || stored[0].Goal == nil {
		t.Fatalf("stored events = %#v", stored)
	}
}
