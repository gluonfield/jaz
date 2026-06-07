package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/provider"
)

func TestProviderStreamsTextReasoningAndToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"content":"hi "},"finish_reason":null}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"reasoning_content":"thinking "},"finish_reason":null}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"exec_","arguments":"{\"cmd\":"}}]},"finish_reason":null}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"command","arguments":"\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := New(server.URL, "test-key", "test-model")
	stream, err := p.StreamComplete(context.Background(), provider.Request{Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}

	var delta string
	var reasoning string
	var call *provider.ToolCall
	for event := range stream {
		if event.Type == provider.EventError {
			t.Fatalf("provider error: %v", event.Err)
		}
		if event.Type == provider.EventDelta {
			delta += event.Delta
		}
		if event.Type == provider.EventReasoning {
			reasoning += event.Reasoning
		}
		if event.Type == provider.EventToolCall {
			call = event.ToolCall
		}
	}
	if delta != "hi " {
		t.Fatalf("unexpected delta %q", delta)
	}
	if reasoning != "thinking " {
		t.Fatalf("unexpected reasoning %q", reasoning)
	}
	if call == nil || provider.ToolCallName(*call) != "exec_command" || provider.ToolCallArguments(*call) != `{"cmd":"pwd"}` {
		t.Fatalf("unexpected call %#v", call)
	}
}
