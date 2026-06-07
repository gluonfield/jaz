package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/tools"
)

const (
	StreamDelta      = "delta"
	StreamReasoning  = "reasoning"
	StreamToolCall   = "tool_call"
	StreamToolResult = "tool_result"
	StreamTurn       = "turn"
	StreamError      = "error"
	StreamDone       = "done"
)

var DefaultMaxTurns = 25

type StreamEvent struct {
	Type               string             `json:"type"`
	Delta              string             `json:"delta,omitempty"`
	Reasoning          string             `json:"reasoning,omitempty"`
	ToolCall           *provider.ToolCall `json:"tool_call,omitempty"`
	ToolName           string             `json:"tool_name,omitempty"`
	Result             string             `json:"result,omitempty"`
	Error              string             `json:"error,omitempty"`
	Usage              *provider.Usage    `json:"usage,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
	At                 time.Time          `json:"at"`
	Messages           []provider.Message `json:"-"`
	ReasoningByMessage map[int]string     `json:"-"`
}

type Agent struct {
	Provider provider.Provider
	Model    string
	Tools    *tools.Registry
	MaxTurns int
}

type Result struct {
	Message        provider.Message   `json:"message"`
	Messages       []provider.Message `json:"messages"`
	Content        string             `json:"content"`
	Usage          provider.Usage     `json:"usage,omitempty"`
	ToolExecutions []ToolExecution    `json:"tool_executions,omitempty"`
}

type ToolExecution struct {
	Call   provider.ToolCall `json:"call"`
	Result string            `json:"result"`
}

func (a *Agent) Complete(ctx context.Context, req provider.Request) (Result, error) {
	req, err := a.normalize(req)
	if err != nil {
		return Result{}, err
	}
	messages := append([]provider.Message(nil), req.Messages...)
	var toolExecutions []ToolExecution
	var usage provider.Usage

	for turn := 0; turn < a.MaxTurns; turn++ {
		resp, err := a.Provider.Complete(ctx, provider.Request{
			Model:    req.Model,
			Messages: messages,
			Tools:    req.Tools,
		})
		if err != nil {
			return Result{Messages: messages, ToolExecutions: toolExecutions}, err
		}
		usage = addUsage(usage, resp.Usage)

		calls := provider.MessageToolCalls(resp.Message)
		messages = append(messages, resp.Message)
		if len(calls) == 0 {
			return Result{
				Message:        resp.Message,
				Messages:       messages,
				Content:        provider.MessageContent(resp.Message),
				Usage:          usage,
				ToolExecutions: toolExecutions,
			}, nil
		}
		for _, call := range calls {
			result := a.executeTool(ctx, call)
			messages = append(messages, provider.ToolMessage(result, provider.ToolCallID(call)))
			toolExecutions = append(toolExecutions, ToolExecution{Call: call, Result: result})
		}
	}
	return Result{Messages: messages, ToolExecutions: toolExecutions}, fmt.Errorf("stopped after %d tool turns", a.MaxTurns)
}

func (a *Agent) Run(ctx context.Context, req provider.Request) <-chan StreamEvent {
	out := make(chan StreamEvent)
	go func() {
		defer close(out)
		a.run(ctx, req, out)
	}()
	return out
}

func (a *Agent) run(ctx context.Context, req provider.Request, out chan<- StreamEvent) {
	req, err := a.normalize(req)
	if err != nil {
		a.emit(out, StreamEvent{Type: StreamError, Error: err.Error()})
		return
	}
	messages := append([]provider.Message(nil), req.Messages...)
	var usage provider.Usage
	reasoningByMessage := map[int]string{}

	for turn := 0; turn < a.MaxTurns; turn++ {
		stream, err := a.Provider.StreamComplete(ctx, provider.Request{
			Model:    req.Model,
			Messages: messages,
			Tools:    req.Tools,
		})
		if err != nil {
			a.emit(out, StreamEvent{Type: StreamError, Error: err.Error(), Messages: messages})
			return
		}

		var assistantText strings.Builder
		var reasoningText strings.Builder
		var calls []provider.ToolCall
		for event := range stream {
			switch event.Type {
			case provider.EventDelta:
				assistantText.WriteString(event.Delta)
				a.emit(out, StreamEvent{Type: StreamDelta, Delta: event.Delta})
			case provider.EventReasoning:
				reasoningText.WriteString(event.Reasoning)
				a.emit(out, StreamEvent{Type: StreamReasoning, Reasoning: event.Reasoning})
			case provider.EventToolCall:
				if event.ToolCall != nil {
					calls = append(calls, *event.ToolCall)
					a.emit(out, StreamEvent{Type: StreamToolCall, ToolCall: event.ToolCall})
				}
			case provider.EventError:
				msg := "provider error"
				if event.Err != nil {
					msg = event.Err.Error()
				}
				a.emit(out, StreamEvent{Type: StreamError, Error: msg, Messages: messages})
				return
			case provider.EventDone:
				usage = addUsage(usage, event.Usage)
			}
		}

		if len(calls) == 0 {
			messages = append(messages, provider.AssistantMessage(assistantText.String(), nil))
			recordReasoning(reasoningByMessage, len(messages)-1, reasoningText.String())
			a.emit(out, StreamEvent{Type: StreamDone, Messages: messages, ReasoningByMessage: reasoningByMessage, Usage: &usage})
			return
		}

		messages = append(messages, provider.AssistantMessage(assistantText.String(), calls))
		recordReasoning(reasoningByMessage, len(messages)-1, reasoningText.String())
		// Snapshot pre-execution so the round is stamped when the model produced it.
		a.emit(out, snapshotEvent(messages, reasoningByMessage))
		for _, call := range calls {
			result := a.executeTool(ctx, call)
			messages = append(messages, provider.ToolMessage(result, provider.ToolCallID(call)))
			a.emit(out, StreamEvent{
				Type:     StreamToolResult,
				ToolName: provider.ToolCallName(call),
				Result:   result,
			})
		}
		a.emit(out, snapshotEvent(messages, reasoningByMessage))
	}

	errMsg := fmt.Sprintf("stopped after %d tool turns", a.MaxTurns)
	a.emit(out, StreamEvent{Type: StreamError, Error: errMsg, Messages: messages})
}

func recordReasoning(out map[int]string, index int, reasoning string) {
	if strings.TrimSpace(reasoning) != "" {
		out[index] = reasoning
	}
}

// Copies state so the consumer can persist it while the run keeps appending.
func snapshotEvent(messages []provider.Message, reasoningByMessage map[int]string) StreamEvent {
	reasoning := make(map[int]string, len(reasoningByMessage))
	maps.Copy(reasoning, reasoningByMessage)
	return StreamEvent{
		Type:               StreamTurn,
		Messages:           append([]provider.Message(nil), messages...),
		ReasoningByMessage: reasoning,
	}
}

func (a *Agent) normalize(req provider.Request) (provider.Request, error) {
	if a.Provider == nil {
		return req, fmt.Errorf("provider is nil")
	}
	if a.Tools == nil {
		a.Tools = tools.NewRegistry()
	}
	if a.MaxTurns <= 0 {
		a.MaxTurns = DefaultMaxTurns
	}
	if req.Model == "" {
		req.Model = a.Model
	}
	req.Messages = append([]provider.Message(nil), req.Messages...)
	if len(req.Tools) == 0 {
		req.Tools = a.Tools.Definitions()
	}
	return req, nil
}

func (a *Agent) executeTool(ctx context.Context, call provider.ToolCall) string {
	name := provider.ToolCallName(call)
	tool, ok := a.Tools.Get(name)
	if !ok {
		return marshalToolError(fmt.Sprintf("unknown tool %q", name))
	}
	inputs := map[string]any{}
	if args := provider.ToolCallArguments(call); strings.TrimSpace(args) != "" {
		if err := json.Unmarshal([]byte(args), &inputs); err != nil {
			return marshalToolError("invalid tool arguments: " + err.Error())
		}
	}
	result, err := tool.Execute(ctx, inputs)
	if err != nil {
		return marshalToolError(err.Error())
	}
	if result.Content == "" {
		return "{}"
	}
	return result.Content
}

func marshalToolError(msg string) string {
	data, err := json.Marshal(map[string]any{
		"status": "error",
		"error":  msg,
	})
	if err != nil {
		return `{"status":"error","error":"tool failed"}`
	}
	return string(data)
}

func (a *Agent) emit(out chan<- StreamEvent, event StreamEvent) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	out <- event
}

func addUsage(a, b provider.Usage) provider.Usage {
	a.InputTokens += b.InputTokens
	a.CachedInputTokens += b.CachedInputTokens
	a.OutputTokens += b.OutputTokens
	a.ReasoningOutputTokens += b.ReasoningOutputTokens
	a.TotalTokens += b.TotalTokens
	return a
}
