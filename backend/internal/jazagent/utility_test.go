package jazagent

import (
	"context"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"github.com/wins/jaz/backend/internal/tools"
)

type utilityProvider struct {
	requests []provider.Request
}

func (p *utilityProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{}, nil
}

func (p *utilityProvider) StreamComplete(_ context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.requests = append(p.requests, req)
	out := make(chan provider.Event, 2)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventDelta, Delta: "Utility Title"}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out, nil
}

type utilityTool struct{}

func (utilityTool) Definition() tools.Definition {
	return tools.Function("utility_tool", "should not be exposed", false, map[string]any{"type": "object"})
}

func (utilityTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	return tools.Result{Content: "{}"}, nil
}

type staticPrompt string

func (p staticPrompt) SystemPromptForWorkspace(string) (string, error) { return string(p), nil }

func TestBuildRequestAddsSystemPromptExtensionWithoutChangingUserMessage(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug: "loop-run",
	})
	if err != nil {
		t.Fatal(err)
	}

	turn, err := BuildRequest(store, staticPrompt("base prompt"), Request{
		Session:                session,
		Message:                "news In AI",
		AppendUser:             true,
		SystemPromptExtensions: []string{"run context"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := provider.MessageContent(turn.Messages[0]); got != "base prompt\n\nrun context" {
		t.Fatalf("system prompt = %q", got)
	}
	if got := provider.MessageContent(turn.Messages[len(turn.Messages)-1]); got != "news In AI" {
		t.Fatalf("user prompt = %q", got)
	}
	stored, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || provider.MessageContent(stored[0]) != "news In AI" {
		t.Fatalf("stored messages = %#v", stored)
	}
}

func TestRunUtilityDoesNotUseTranscriptStoreOrTools(t *testing.T) {
	fp := &utilityProvider{}
	runner := &Runner{
		Agent: &agent.Agent{
			Provider: fp,
			Tools:    tools.NewRegistry(utilityTool{}),
		},
	}
	session := storage.Session{
		ID:              "utility",
		ModelProvider:   "openrouter",
		Model:           "openai/gpt-test",
		ReasoningEffort: "medium",
		RuntimeRef: &storage.RuntimeRef{
			Cwd: t.TempDir(),
		},
	}

	var text strings.Builder
	for event := range runner.RunUtility(context.Background(), UtilityRequest{
		Session: session,
		Message: "name this thread",
	}) {
		if event.Type == agent.StreamError {
			t.Fatal(event.Error)
		}
		if event.Type == agent.StreamDelta {
			text.WriteString(event.Delta)
		}
	}

	if text.String() != "Utility Title" {
		t.Fatalf("text = %q, want utility output", text.String())
	}
	if len(fp.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(fp.requests))
	}
	req := fp.requests[0]
	if req.Provider != "openrouter" || req.Model != "openai/gpt-test" || req.ReasoningEffort != "medium" {
		t.Fatalf("request model config = %#v", req)
	}
	if len(req.Tools) != 0 {
		t.Fatalf("tools = %#v, want no utility tools", req.Tools)
	}
	if len(req.Messages) != 1 || provider.MessageContent(req.Messages[0]) != "name this thread" {
		t.Fatalf("messages = %#v", req.Messages)
	}
}
