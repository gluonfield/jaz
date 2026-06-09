package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestProviderCompleteTranslatesMessagesToolsAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "test-key" || r.Header.Get("Anthropic-Version") != apiVersion {
			t.Fatalf("unexpected headers: %#v", r.Header)
		}
		var req messageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "claude-test" || req.System != "system prompt" {
			t.Fatalf("unexpected request model/system: %#v", req)
		}
		if req.Thinking == nil || req.Thinking.BudgetTokens != 2048 {
			t.Fatalf("thinking = %#v, want medium budget", req.Thinking)
		}
		if len(req.Tools) != 1 || req.Tools[0].Name != "exec_command" || req.Tools[0].InputSchema["type"] != "object" {
			t.Fatalf("unexpected tools: %#v", req.Tools)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content[0].Text != "hello" {
			t.Fatalf("unexpected messages: %#v", req.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "text", "text": "done"},
				{"type": "tool_use", "id": "toolu_1", "name": "exec_command", "input": {"cmd": "pwd"}}
			],
			"usage": {"input_tokens": 10, "cache_read_input_tokens": 4, "output_tokens": 5}
		}`)
	}))
	defer server.Close()

	p := New(server.URL, "test-key", "claude-test")
	resp, err := p.Complete(context.Background(), provider.Request{
		ReasoningEffort: "medium",
		Messages: []provider.Message{
			provider.SystemMessage("system prompt"),
			provider.UserMessage("hello"),
		},
		Tools: []tools.Definition{
			tools.Function("exec_command", "Run a command", true, tools.ObjectSchema(map[string]any{
				"cmd": tools.StringSchema("Command"),
			}, []string{"cmd"})),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := provider.MessageToolCalls(resp.Message)
	if provider.MessageContent(resp.Message) != "done" || len(calls) != 1 ||
		provider.ToolCallName(calls[0]) != "exec_command" || provider.ToolCallArguments(calls[0]) != `{"cmd":"pwd"}` {
		t.Fatalf("unexpected message: content=%q calls=%#v", provider.MessageContent(resp.Message), calls)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.CachedInputTokens != 4 ||
		resp.Usage.OutputTokens != 5 || resp.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
}

func TestProviderStreamMapsDeltasThinkingToolCallsAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req messageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if !req.Stream {
			t.Fatalf("stream = false, want true")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":12}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"think "}}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_2","name":"exec_command","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"pwd\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`)
	}))
	defer server.Close()

	p := New(server.URL, "test-key", "claude-test")
	stream, err := p.StreamComplete(context.Background(), provider.Request{Messages: []provider.Message{provider.UserMessage("hello")}})
	if err != nil {
		t.Fatal(err)
	}
	var delta string
	var reasoning string
	var call *provider.ToolCall
	var usage provider.Usage
	for event := range stream {
		switch event.Type {
		case provider.EventError:
			t.Fatalf("provider error: %v", event.Err)
		case provider.EventDelta:
			delta += event.Delta
		case provider.EventReasoning:
			reasoning += event.Reasoning
		case provider.EventToolCall:
			call = event.ToolCall
		case provider.EventDone:
			usage = event.Usage
		}
	}
	if delta != "hi " || reasoning != "think " {
		t.Fatalf("delta=%q reasoning=%q", delta, reasoning)
	}
	if call == nil || provider.ToolCallName(*call) != "exec_command" || provider.ToolCallArguments(*call) != `{"cmd":"pwd"}` {
		t.Fatalf("unexpected tool call %#v", call)
	}
	if usage.InputTokens != 12 || usage.OutputTokens != 5 || usage.TotalTokens != 17 {
		t.Fatalf("usage = %#v", usage)
	}
}
