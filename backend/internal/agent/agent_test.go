package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/tools"
)

type fakeProvider struct {
	calls    int
	requests []provider.Request
}

func (p *fakeProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	p.calls++
	p.requests = append(p.requests, req)
	if p.calls == 1 {
		return provider.Response{Message: provider.AssistantMessage("", []provider.ToolCall{
			provider.FunctionToolCall("call_1", "mock", `{"value":"ok"}`),
		})}, nil
	}
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *fakeProvider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.calls++
	p.requests = append(p.requests, req)
	ch := make(chan provider.Event, 4)
	go func() {
		defer close(ch)
		if p.calls == 1 {
			call := provider.FunctionToolCall("call_1", "mock", `{"value":"ok"}`)
			ch <- provider.Event{Type: provider.EventToolCall, ToolCall: &call}
			ch <- provider.Event{Type: provider.EventDone}
			return
		}
		ch <- provider.Event{Type: provider.EventDelta, Delta: "done"}
		ch <- provider.Event{Type: provider.EventDone}
	}()
	return ch, nil
}

type mockTool struct{}

func (mockTool) Definition() tools.Definition {
	return tools.Function("mock", "mock tool", false, map[string]any{"type": "object"})
}

func (mockTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	return tools.Result{Content: `{"status":"completed","value":"` + inputs["value"].(string) + `"}`}, nil
}

func TestAgentToolLoop(t *testing.T) {
	a := &Agent{
		Provider: &fakeProvider{},
		Tools:    tools.NewRegistry(mockTool{}),
	}

	var text strings.Builder
	var sawTool bool
	messages := []provider.Message{provider.UserMessage("hello")}
	for event := range a.Run(context.Background(), provider.Request{Messages: messages}) {
		if event.Type == StreamToolResult {
			sawTool = true
		}
		if event.Type == StreamDelta {
			text.WriteString(event.Delta)
		}
	}
	if !sawTool {
		t.Fatal("expected tool result event")
	}
	if text.String() != "done" {
		t.Fatalf("unexpected streamed text %q", text.String())
	}
}

func TestAgentCompleteReturnsFinalResult(t *testing.T) {
	a := &Agent{
		Provider: &fakeProvider{},
		Tools:    tools.NewRegistry(mockTool{}),
	}
	result, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{provider.UserMessage("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "done" {
		t.Fatalf("unexpected content %q", result.Content)
	}
	if len(result.ToolExecutions) != 1 || provider.ToolCallName(result.ToolExecutions[0].Call) != "mock" {
		t.Fatalf("unexpected tool executions %#v", result.ToolExecutions)
	}
	if len(result.Messages) != 4 {
		t.Fatalf("expected user, assistant-call, tool, assistant messages; got %d", len(result.Messages))
	}
}

func TestAgentUsesDefaultReasoningEffort(t *testing.T) {
	fp := &fakeProvider{}
	a := &Agent{
		Provider:        fp,
		ModelProvider:   "openrouter",
		ReasoningEffort: "high",
		Tools:           tools.NewRegistry(mockTool{}),
	}
	_, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{provider.UserMessage("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.requests) == 0 || fp.requests[0].Provider != "openrouter" || fp.requests[0].ReasoningEffort != "high" {
		t.Fatalf("provider defaults were not forwarded: %#v", fp.requests)
	}
}
