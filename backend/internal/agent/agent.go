package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/media"
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
	Type               string                 `json:"type"`
	Delta              string                 `json:"delta,omitempty"`
	Reasoning          string                 `json:"reasoning,omitempty"`
	ToolCall           *provider.ToolCall     `json:"tool_call,omitempty"`
	ToolName           string                 `json:"tool_name,omitempty"`
	Result             string                 `json:"result,omitempty"`
	Error              string                 `json:"error,omitempty"`
	Usage              *provider.Usage        `json:"usage,omitempty"`
	Metadata           map[string]any         `json:"metadata,omitempty"`
	At                 time.Time              `json:"at"`
	Messages           []provider.Message     `json:"-"`
	MediaRefs          map[string][]media.Ref `json:"-"`
	ReasoningByMessage map[int]string         `json:"-"`
}

type Agent struct {
	Provider        provider.Provider
	ModelProvider   string
	Model           string
	ReasoningEffort string
	Tools           *tools.Registry
	MaxTurns        int
}

type Result struct {
	Message        provider.Message       `json:"message"`
	Messages       []provider.Message     `json:"messages"`
	Content        string                 `json:"content"`
	Usage          provider.Usage         `json:"usage,omitempty"`
	ToolExecutions []ToolExecution        `json:"tool_executions,omitempty"`
	MediaRefs      map[string][]media.Ref `json:"-"`
}

type ToolExecution struct {
	Call      provider.ToolCall `json:"call"`
	Result    string            `json:"result"`
	MediaRefs []media.Ref       `json:"-"`
}

func (a *Agent) Complete(ctx context.Context, req provider.Request) (Result, error) {
	req, err := a.normalize(req)
	if err != nil {
		return Result{}, err
	}
	messages := append([]provider.Message(nil), req.Messages...)
	mediaRefs := media.CloneRefMap(req.MediaRefs)
	var toolExecutions []ToolExecution
	var usage provider.Usage

	for turn := 0; turn < a.MaxTurns; turn++ {
		requestMessages, err := requestMessagesWithMediaRefs(messages, mediaRefs)
		if err != nil {
			return Result{Messages: messages, ToolExecutions: toolExecutions, Usage: usage, MediaRefs: mediaRefs}, err
		}
		resp, err := a.Provider.Complete(ctx, provider.Request{
			Provider:        req.Provider,
			Model:           req.Model,
			ReasoningEffort: req.ReasoningEffort,
			Messages:        requestMessages,
			Tools:           req.Tools,
		})
		if err != nil {
			return Result{Messages: messages, ToolExecutions: toolExecutions, MediaRefs: mediaRefs}, err
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
				MediaRefs:      mediaRefs,
			}, nil
		}
		for _, call := range calls {
			result := a.executeTool(ctx, call)
			callID := provider.ToolCallID(call)
			if len(result.MediaRefs) > 0 {
				mediaRefs = withMediaRefs(mediaRefs, callID, result.MediaRefs)
			}
			messages = append(messages, provider.ToolMessage(result.Content, callID))
			toolExecutions = append(toolExecutions, ToolExecution{Call: call, Result: result.Content, MediaRefs: media.CloneRefs(result.MediaRefs)})
		}
	}
	return Result{Messages: messages, ToolExecutions: toolExecutions, MediaRefs: mediaRefs}, fmt.Errorf("stopped after %d tool turns", a.MaxTurns)
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
	mediaRefs := media.CloneRefMap(req.MediaRefs)
	var usage provider.Usage
	reasoningByMessage := map[int]string{}

	for turn := 0; turn < a.MaxTurns; turn++ {
		requestMessages, err := requestMessagesWithMediaRefs(messages, mediaRefs)
		if err != nil {
			a.emit(out, StreamEvent{Type: StreamError, Error: err.Error(), Messages: messages, MediaRefs: mediaRefs})
			return
		}
		stream, err := a.Provider.StreamComplete(ctx, provider.Request{
			Provider:        req.Provider,
			Model:           req.Model,
			ReasoningEffort: req.ReasoningEffort,
			Messages:        requestMessages,
			Tools:           req.Tools,
		})
		if err != nil {
			a.emit(out, StreamEvent{Type: StreamError, Error: err.Error(), Messages: messages, MediaRefs: mediaRefs})
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
				a.emit(out, StreamEvent{Type: StreamError, Error: msg, Messages: messages, MediaRefs: mediaRefs})
				return
			case provider.EventDone:
				usage = addUsage(usage, event.Usage)
			}
		}

		if len(calls) == 0 {
			messages = append(messages, provider.AssistantMessage(assistantText.String(), nil))
			recordReasoning(reasoningByMessage, len(messages)-1, reasoningText.String())
			a.emit(out, StreamEvent{Type: StreamDone, Messages: messages, MediaRefs: mediaRefs, ReasoningByMessage: reasoningByMessage, Usage: &usage})
			return
		}

		messages = append(messages, provider.AssistantMessage(assistantText.String(), calls))
		recordReasoning(reasoningByMessage, len(messages)-1, reasoningText.String())
		// Snapshot pre-execution so the round is stamped when the model produced it.
		a.emit(out, snapshotEvent(messages, reasoningByMessage, mediaRefs))
		for _, call := range calls {
			result := a.executeTool(ctx, call)
			callID := provider.ToolCallID(call)
			if len(result.MediaRefs) > 0 {
				mediaRefs = withMediaRefs(mediaRefs, callID, result.MediaRefs)
			}
			messages = append(messages, provider.ToolMessage(result.Content, callID))
			event := StreamEvent{
				Type:     StreamToolResult,
				ToolName: provider.ToolCallName(call),
				Result:   result.Content,
				Metadata: result.Metadata,
			}
			if len(result.MediaRefs) > 0 {
				event.MediaRefs = map[string][]media.Ref{callID: media.CloneRefs(result.MediaRefs)}
			}
			a.emit(out, event)
		}
		a.emit(out, snapshotEvent(messages, reasoningByMessage, mediaRefs))
	}

	errMsg := fmt.Sprintf("stopped after %d tool turns", a.MaxTurns)
	a.emit(out, StreamEvent{Type: StreamError, Error: errMsg, Messages: messages, MediaRefs: mediaRefs})
}

func recordReasoning(out map[int]string, index int, reasoning string) {
	if strings.TrimSpace(reasoning) != "" {
		out[index] = reasoning
	}
}

func withMediaRefs(refsByToolCall map[string][]media.Ref, callID string, refs []media.Ref) map[string][]media.Ref {
	if len(refs) == 0 {
		return refsByToolCall
	}
	if refsByToolCall == nil {
		refsByToolCall = map[string][]media.Ref{}
	}
	refsByToolCall[callID] = media.CloneRefs(refs)
	return refsByToolCall
}

// Copies state so the consumer can persist it while the run keeps appending.
func snapshotEvent(messages []provider.Message, reasoningByMessage map[int]string, mediaRefs map[string][]media.Ref) StreamEvent {
	reasoning := make(map[int]string, len(reasoningByMessage))
	maps.Copy(reasoning, reasoningByMessage)
	return StreamEvent{
		Type:               StreamTurn,
		Messages:           append([]provider.Message(nil), messages...),
		MediaRefs:          media.CloneRefMap(mediaRefs),
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
	if req.Provider == "" {
		req.Provider = a.ModelProvider
	}
	if req.Model == "" {
		req.Model = a.Model
	}
	if req.ReasoningEffort == "" {
		req.ReasoningEffort = a.ReasoningEffort
	}
	req.Messages = append([]provider.Message(nil), req.Messages...)
	req.MediaRefs = media.CloneRefMap(req.MediaRefs)
	if len(req.Tools) == 0 {
		req.Tools = a.Tools.Definitions()
	}
	return req, nil
}

func requestMessagesWithMediaRefs(messages []provider.Message, refsByToolCall map[string][]media.Ref) ([]provider.Message, error) {
	out := make([]provider.Message, 0, len(messages))
	var pendingRefs []media.Ref
	for i, msg := range messages {
		out = append(out, msg)
		if provider.MessageRole(msg) != "tool" {
			continue
		}
		pendingRefs = append(pendingRefs, refsByToolCall[provider.MessageToolCallID(msg)]...)
		if i+1 < len(messages) && provider.MessageRole(messages[i+1]) == "tool" {
			continue
		}
		if len(pendingRefs) == 0 {
			continue
		}
		mediaMessage, err := mediaMessageForRefs(pendingRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, mediaMessage)
		pendingRefs = nil
	}
	return out, nil
}

func mediaMessageForRefs(refs []media.Ref) (provider.Message, error) {
	parts := make([]provider.ContentPart, 0, len(refs)*2)
	for _, ref := range refs {
		text := strings.TrimSpace(ref.Text)
		if text == "" {
			text = "Image returned by view_image"
			if ref.Filename != "" {
				text += ": " + ref.Filename
			}
		}
		parts = append(parts, provider.TextPart(text))
		part, err := media.MaterializeRef(ref)
		if err != nil {
			return provider.Message{}, err
		}
		parts = append(parts, provider.ImageURLPart(part.ImageURL, part.Detail))
	}
	return provider.UserMessageParts(parts...), nil
}

func (a *Agent) executeTool(ctx context.Context, call provider.ToolCall) tools.Result {
	name := provider.ToolCallName(call)
	tool, ok := a.Tools.Get(name)
	if !ok {
		return tools.Result{Content: marshalToolError(fmt.Sprintf("unknown tool %q", name))}
	}
	inputs := map[string]any{}
	if args := provider.ToolCallArguments(call); strings.TrimSpace(args) != "" {
		if err := json.Unmarshal([]byte(args), &inputs); err != nil {
			return tools.Result{Content: marshalToolError("invalid tool arguments: " + err.Error())}
		}
	}
	result, err := tool.Execute(ctx, inputs)
	if err != nil {
		return tools.Result{Content: marshalToolError(err.Error())}
	}
	if result.Content == "" {
		result.Content = "{}"
	}
	return result
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
