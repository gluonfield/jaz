package openai

import (
	"context"
	"encoding/json"
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
		var req struct {
			ReasoningEffort string `json:"reasoning_effort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ReasoningEffort != "high" {
			t.Fatalf("reasoning_effort = %q, want high", req.ReasoningEffort)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"content":"hi "},"finish_reason":null}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"reasoning_content":"thinking "},"finish_reason":null}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"exec_","arguments":"{\"cmd\":"}}]},"finish_reason":null}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"command","arguments":"\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"cache_read_input_tokens":80}}`+"\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := New(server.URL, "test-key", "test-model")
	stream, err := p.StreamComplete(context.Background(), provider.Request{Model: "test-model", ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}

	var delta string
	var reasoning string
	var call *provider.ToolCall
	var usage provider.Usage
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
		if event.Type == provider.EventDone {
			usage = event.Usage
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
	if usage.InputTokens != 100 || usage.CachedInputTokens != 80 || usage.OutputTokens != 20 || usage.TotalTokens != 120 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestProviderOmitsReasoningEffortWhenNone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if _, ok := req["reasoning_effort"]; ok {
			t.Fatalf("reasoning_effort should be omitted when configured as none: %#v", req)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := New(server.URL, "test-key", "test-model")
	stream, err := p.StreamComplete(context.Background(), provider.Request{Model: "test-model", ReasoningEffort: "none"})
	if err != nil {
		t.Fatal(err)
	}
	for event := range stream {
		if event.Type == provider.EventError {
			t.Fatalf("provider error: %v", event.Err)
		}
	}
}

func TestProviderCompleteMapsCachedUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "done"},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 128,
				"completion_tokens": 16,
				"total_tokens": 144,
				"cache_read_input_tokens": 96,
				"prompt_tokens_details": {},
				"completion_tokens_details": {"reasoning_tokens": 4}
			}
		}`)
	}))
	defer server.Close()

	p := New(server.URL, "test-key", "test-model")
	resp, err := p.Complete(context.Background(), provider.Request{Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.InputTokens != 128 || resp.Usage.CachedInputTokens != 96 || resp.Usage.OutputTokens != 16 ||
		resp.Usage.ReasoningOutputTokens != 4 || resp.Usage.TotalTokens != 144 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
}

func TestProviderCompleteRequestsOpenRouterUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var req struct {
			Usage struct {
				Include bool `json:"include"`
			} `json:"usage"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if !req.Usage.Include {
			t.Fatalf("usage.include = false, want true")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "done"},
				"finish_reason": "stop"
			}]
		}`)
	}))
	defer server.Close()

	p := New(server.URL, "test-key", "test-model")
	p.IncludeUsage = true
	if _, err := p.Complete(context.Background(), provider.Request{Model: "test-model"}); err != nil {
		t.Fatal(err)
	}
}
