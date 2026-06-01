package mock

import (
	"context"

	"github.com/wins/jaz/backend/internal/provider"
)

type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if lastRole(req) == "tool" {
		return provider.Response{Message: provider.AssistantMessage("Mock provider received the tool result and finished.", nil)}, nil
	}
	return provider.Response{Message: provider.AssistantMessage(
		"Mock provider is calling exec_command.\n",
		[]provider.ToolCall{mockCall()},
	)}, nil
}

func (p *Provider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 4)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
			ch <- provider.Event{Type: provider.EventError, Err: ctx.Err()}
			return
		default:
		}

		if lastRole(req) == "tool" {
			ch <- provider.Event{Type: provider.EventDelta, Delta: "Mock provider received the tool result and finished."}
			ch <- provider.Event{Type: provider.EventDone}
			return
		}

		ch <- provider.Event{Type: provider.EventDelta, Delta: "Mock provider is calling exec_command.\n"}
		ch <- provider.Event{
			Type:     provider.EventToolCall,
			ToolCall: ptr(mockCall()),
		}
		ch <- provider.Event{Type: provider.EventDone}
	}()
	return ch, nil
}

func lastRole(req provider.Request) string {
	if len(req.Messages) == 0 {
		return ""
	}
	return provider.MessageRole(req.Messages[len(req.Messages)-1])
}

func mockCall() provider.ToolCall {
	return provider.FunctionToolCall("call_mock_exec", "exec_command", `{"cmd":"printf mock-tool-ok","yield_time_ms":1000}`)
}

func ptr[T any](v T) *T {
	return &v
}
