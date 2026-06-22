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

func TestPlanRequestedTextPublishesProposedPlan(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{Slug: "parent", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:     "codex-plan",
		Runtime:  storage.RuntimeACP,
		ParentID: parent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	job := &Job{
		ID:            session.ID,
		ParentID:      parent.ID,
		ParentVisible: true,
		ACPAgent:      AgentCodex,
		ACPSession:    "acp-session",
		Cwd:           t.TempDir(),
		toolByID:      map[string]ToolCallSnapshot{},
	}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job
	done := job.startTurn(CompletionInline, true, true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	childSub := events.Subscribe(ctx, session.ID)
	parentSub := events.Subscribe(ctx, parent.ID)

	raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": "<proposed_plan>\nInspect the provider path.\n</proposed_plan>",
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if len(raw) == 0 {
		t.Fatal("empty update response")
	}
	assertNoEvent(t, childSub)

	job.setState(StateIdle, "", "")
	manager.finishTurn(done, job)

	childEvent := receiveEvent(t, childSub)
	if childEvent.Type != "proposed_plan" || childEvent.Plan == nil {
		t.Fatalf("child event = %#v", childEvent)
	}
	if !childEvent.Plan.AwaitingApproval || childEvent.Plan.Explanation != "Inspect the provider path." {
		t.Fatalf("child plan = %#v", childEvent.Plan)
	}
	if childEvent.ACP == nil || childEvent.ACP.ID != session.ID {
		t.Fatalf("child acp envelope = %#v", childEvent.ACP)
	}

	parentEvent := receiveEvent(t, parentSub)
	if parentEvent.Type != "proposed_plan" || parentEvent.SessionID != parent.ID {
		t.Fatalf("parent event = %#v", parentEvent)
	}
	if parentEvent.ACP == nil || parentEvent.ACP.ID != session.ID || parentEvent.ACP.ParentID != parent.ID {
		t.Fatalf("parent acp envelope = %#v", parentEvent.ACP)
	}

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Type != "proposed_plan" {
		t.Fatalf("stored events = %#v", stored)
	}
}

func TestPlanRequestedPlainTextPublishesACPMessage(t *testing.T) {
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
	job := &Job{
		ID:         session.ID,
		ACPAgent:   AgentCodex,
		ACPSession: "acp-session",
		Cwd:        t.TempDir(),
		toolByID:   map[string]ToolCallSnapshot{},
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
					"text": "I need one more detail before proposing a plan.",
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

	event := receiveEvent(t, sub)
	if event.Type != "acp_message" || event.Content != "I need one more detail before proposing a plan." {
		t.Fatalf("event = %#v", event)
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
	job := &Job{
		ID:         session.ID,
		ACPAgent:   AgentCodex,
		ACPSession: "acp-session",
		Cwd:        t.TempDir(),
		toolByID:   map[string]ToolCallSnapshot{},
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

func TestLocalPlanRequestedPlainTextPublishesACPMessage(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "local-plan", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	job := &Job{
		ID:         session.ID,
		ACPAgent:   AgentJaz,
		ACPSession: session.ID,
		Cwd:        t.TempDir(),
		toolByID:   map[string]ToolCallSnapshot{},
	}
	manager.jobsByID[session.ID] = job
	done := job.startTurn(CompletionInline, true, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	manager.applyLocalMessage(job, "local reply")
	assertNoEvent(t, sub)

	job.setState(StateIdle, "", "")
	manager.finishTurn(done, job)

	proposal := receiveEvent(t, sub)
	if proposal.Type != "acp_message" || proposal.Content != "local reply" {
		t.Fatalf("event = %#v", proposal)
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
