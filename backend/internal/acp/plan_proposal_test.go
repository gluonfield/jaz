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

func TestCodexPlanRequestedTextDoesNotPublishProposedPlan(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "plain-plan-text", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	job := &jobState{
		Job: Job{
			ID:         session.ID,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			Cwd:        t.TempDir(),
		},
		toolByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job
	done := job.startTurn(CompletionInline, true, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": "Proposed plan:\n- Inspect the current app structure.\n- Build the dinosaur page.\n- Run the relevant checks.",
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	assertNoEvent(t, sub)

	job.setState(StateIdle, "", "")
	manager.finishTurn(done, job)
	assertNoEvent(t, sub)
}

func TestCodexPlanRequestedPreambleDoesNotPublishProposedPlan(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "preamble-plan-text", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	job := &jobState{
		Job: Job{
			ID:         session.ID,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			Cwd:        t.TempDir(),
		},
		toolByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job
	done := job.startTurn(CompletionInline, true, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": "I’ll first check what’s in the current workspace so the plan fits the actual project shape.",
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	assertNoEvent(t, sub)

	job.setState(StateIdle, "", "")
	manager.finishTurn(done, job)
	assertNoEvent(t, sub)
}

func TestCodexPlanRequestedPlanDocumentPublishesProposedPlan(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "plan-document", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	job := &jobState{
		Job: Job{
			ID:         session.ID,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			Cwd:        t.TempDir(),
		},
		toolByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job
	done := job.startTurn(CompletionInline, true, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	planText := "# Plan\n\n- Inspect the current app structure.\n- Build the dinosaur page.\n- Run the relevant checks."
	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "plan",
				"entries": []map[string]any{{
					"content":  planText,
					"status":   "completed",
					"priority": "medium",
				}},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	assertNoEvent(t, sub)

	job.setState(StateIdle, "", "")
	manager.finishTurn(done, job)

	proposal := receiveEvent(t, sub)
	if proposal.Type != "proposed_plan" || proposal.Plan == nil || !proposal.Plan.AwaitingApproval {
		t.Fatalf("proposal event = %#v", proposal)
	}
	if proposal.Plan.Explanation != planText || len(proposal.Plan.Plan) != 0 {
		t.Fatalf("proposal plan = %#v", proposal.Plan)
	}
}

func TestPlanRequestedProgressPublishesProposedPlan(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "structured-plan", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	job := &jobState{
		Job: Job{
			ID:         session.ID,
			ACPAgent:   AgentCodex,
			ACPSession: "acp-session",
			Cwd:        t.TempDir(),
		},
		toolByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job
	done := job.startTurn(CompletionInline, true, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "plan",
				"entries": []map[string]any{
					{"content": "Inspect request", "priority": "high", "status": "completed"},
					{"content": "Wait for approval", "priority": "medium", "status": "in_progress"},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	progress := receiveEvent(t, sub)
	if progress.Type != "acp" || progress.ACP == nil || len(progress.ACP.Plan) != 2 {
		t.Fatalf("progress event = %#v", progress)
	}

	job.setState(StateIdle, "", "")
	manager.finishTurn(done, job)

	proposal := receiveEvent(t, sub)
	if proposal.Type != "proposed_plan" || proposal.Plan == nil || !proposal.Plan.AwaitingApproval {
		t.Fatalf("proposal event = %#v", proposal)
	}
	if len(proposal.Plan.Plan) != 2 ||
		proposal.Plan.Plan[0].Content != "Inspect request" ||
		proposal.Plan.Plan[1].Content != "Wait for approval" {
		t.Fatalf("proposal plan = %#v", proposal.Plan)
	}
	if proposal.ACP == nil || proposal.ACP.Plan != nil {
		t.Fatalf("proposal acp envelope should not carry progress plan: %#v", proposal.ACP)
	}
}

// Claude surfaces plan approval inline via the ExitPlanMode permission, so it
// streams its plan and implementation text live rather than buffering the turn
// for a synthesized proposed_plan.
func TestClaudePlanRequestedStreamsAssistantTextLive(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-plan-live", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	job := &jobState{
		Job: Job{
			ID:         session.ID,
			ACPAgent:   AgentClaude,
			ACPSession: "acp-session",
			Cwd:        t.TempDir(),
		},
		toolByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job
	job.startTurn(CompletionInline, true, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": "Here is the plan I propose."},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	event := receiveEvent(t, sub)
	if event.Type != "acp_message" || event.Content != "Here is the plan I propose." {
		t.Fatalf("expected live acp_message during claude plan turn, got %#v", event)
	}
}

func receiveEvent(t *testing.T, ch <-chan sessionevents.Event) sessionevents.Event {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(time.Second):
		t.Fatal("no event published")
	}
	return sessionevents.Event{}
}

func assertNoEvent(t *testing.T, ch <-chan sessionevents.Event) {
	t.Helper()
	select {
	case event := <-ch:
		t.Fatalf("unexpected event %#v", event)
	default:
	}
}
