package acp

import (
	"context"
	"strings"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

type planTurnFixture struct {
	manager *Manager
	job     *jobState
	done    chan struct{}
	ctx     context.Context
	events  <-chan sessionevents.Event
}

func newPlanTurnFixture(t *testing.T, agent string) planTurnFixture {
	t.Helper()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "plan-turn", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	job := &jobState{
		Job: Job{
			ID:         session.ID,
			ACPAgent:   agent,
			ACPSession: "acp-session",
			Cwd:        t.TempDir(),
		},
		toolByID: map[string]sessionevents.ACPToolCall{},
	}
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job
	done := job.startTurn(CompletionInline, true, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return planTurnFixture{manager: manager, job: job, done: done, ctx: ctx, events: events.Subscribe(ctx, session.ID)}
}

func (f planTurnFixture) update(t *testing.T, update map[string]any) {
	t.Helper()
	_, rpcErr := f.manager.handleJSONRPC(f.ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update":    update,
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
}

func (f planTurnFixture) finish() {
	f.job.setState(StateIdle, "", "")
	f.manager.finishTurn(f.done, f.job)
}

func TestCodexPlanRequestedTextStreamsWithoutBecomingProposedPlan(t *testing.T) {
	f := newPlanTurnFixture(t, AgentCodex)

	f.update(t, map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"messageId":     "codex-plan-proposal",
		"content": map[string]any{
			"type": "text",
			"text": "Proposed plan:\n- Inspect the current app structure.\n- Build the dinosaur page.\n- Run the relevant checks.",
		},
	})
	event := receiveEvent(t, f.events)
	if event.Type != "acp_message" || event.Content != "Proposed plan:\n- Inspect the current app structure.\n- Build the dinosaur page.\n- Run the relevant checks." {
		t.Fatalf("message event = %#v", event)
	}

	f.finish()
	assertNoEvent(t, f.events)
}

func TestCodexPlanRequestedTextPrecedesClarifyingQuestion(t *testing.T) {
	f := newPlanTurnFixture(t, AgentCodex)

	f.update(t, map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"messageId":     "codex-plan-preamble",
		"content": map[string]any{
			"type": "text",
			"text": "I’ll first check what’s in the current workspace so the plan fits the actual project shape.",
		},
	})
	f.manager.publishPermission(f.job, sessionevents.ACPPermission{
		ID:     "question",
		Title:  "Clarifying question",
		Status: "pending",
	}, "permission_request")

	message := receiveEvent(t, f.events)
	if message.Type != "acp_message" || message.Content != "I’ll first check what’s in the current workspace so the plan fits the actual project shape." {
		t.Fatalf("message event = %#v", message)
	}
	question := receiveEvent(t, f.events)
	if question.Type != "permission_request" || question.Permission == nil || question.Permission.ID != "question" {
		t.Fatalf("permission event = %#v", question)
	}

	f.finish()
	assertNoEvent(t, f.events)
}

func TestLocalPlanRequestedTextStreamsLive(t *testing.T) {
	f := newPlanTurnFixture(t, AgentJaz)

	f.manager.applyLocalMessage(f.job, "Here is what I found before proposing the plan.")
	event := receiveEvent(t, f.events)
	if event.Type != "acp_message" || event.Content != "Here is what I found before proposing the plan." {
		t.Fatalf("message event = %#v", event)
	}
}

func TestPlanRequestedUntypedDocumentDoesNotBecomeProposedPlan(t *testing.T) {
	f := newPlanTurnFixture(t, AgentCodex)

	f.update(t, map[string]any{
		"sessionUpdate": "plan",
		"entries":       []map[string]any{{"content": "# Plan\n\n- Untyped draft", "status": "completed"}},
	})
	assertNoEvent(t, f.events)

	f.finish()
	assertNoEvent(t, f.events)
}

func TestPlanRequestedDocumentShapedProgressDoesNotBecomeProposedPlan(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
	}{
		{name: "multiline", content: "Inspect the project\nwhile preserving the current transport contract."},
		{name: "long", content: strings.Repeat("Inspect the existing transport contract. ", 10)},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newPlanTurnFixture(t, AgentCodex)
			f.update(t, map[string]any{
				"sessionUpdate": "plan",
				"entries":       []map[string]any{{"content": test.content, "status": "in_progress"}},
			})
			assertNoEvent(t, f.events)

			f.finish()
			assertNoEvent(t, f.events)
		})
	}
}

// Claude surfaces plan approval inline via the ExitPlanMode permission, so it
// streams its plan and implementation text live rather than buffering the turn
// for a synthesized proposed_plan.
func TestClaudePlanRequestedStreamsAssistantTextLive(t *testing.T) {
	f := newPlanTurnFixture(t, AgentClaude)

	f.update(t, map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"messageId":     "claude-plan-text",
		"content":       map[string]any{"type": "text", "text": "Here is the plan I propose."},
	})
	event := receiveEvent(t, f.events)
	if event.Type != "acp_message" || event.Content != "Here is the plan I propose." {
		t.Fatalf("expected live acp_message during claude plan turn, got %#v", event)
	}
}

func receiveEvent(t *testing.T, ch <-chan sessionevents.Event) sessionevents.Event {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-ch:
			if event.Type != sessionevents.TypeSession {
				return event
			}
		case <-timer.C:
			t.Fatal("no event published")
			return sessionevents.Event{}
		}
	}
}

func assertNoEvent(t *testing.T, ch <-chan sessionevents.Event) {
	t.Helper()
	for {
		select {
		case event := <-ch:
			if event.Type != sessionevents.TypeSession {
				t.Fatalf("unexpected event %#v", event)
			}
		default:
			return
		}
	}
}
