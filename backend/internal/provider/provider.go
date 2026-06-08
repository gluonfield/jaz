package provider

import (
	"context"
	"fmt"
	"strings"

	oa "github.com/openai/openai-go/v3"
	"github.com/wins/jaz/backend/internal/tools"
)

type Message = oa.ChatCompletionMessageParamUnion
type ToolCall = oa.ChatCompletionMessageToolCallUnion

type Request struct {
	Model           string
	ReasoningEffort string
	Messages        []Message
	Tools           []tools.Definition
}

type Response struct {
	Message Message `json:"message"`
	Usage   Usage
}

type Usage struct {
	InputTokens           int64 `json:"input_tokens,omitempty"`
	CachedInputTokens     int64 `json:"cached_input_tokens,omitempty"`
	OutputTokens          int64 `json:"output_tokens,omitempty"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens,omitempty"`
	TotalTokens           int64 `json:"total_tokens,omitempty"`
}

type EventType string

const (
	EventDelta     EventType = "delta"
	EventReasoning EventType = "reasoning"
	EventToolCall  EventType = "tool_call"
	EventDone      EventType = "done"
	EventError     EventType = "error"
)

type Event struct {
	Type      EventType
	Delta     string
	Reasoning string
	ToolCall  *ToolCall
	Usage     Usage
	Err       error
}

type Provider interface {
	Complete(ctx context.Context, req Request) (Response, error)
	StreamComplete(ctx context.Context, req Request) (<-chan Event, error)
}

func NormalizeReasoningEffort(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "none":
		return "", nil
	case "minimal", "low", "medium", "high":
		return value, nil
	default:
		return "", fmt.Errorf("unknown reasoning effort %q; valid values are none, minimal, low, medium, high", value)
	}
}

func SystemMessage(content string) Message {
	return oa.SystemMessage(content)
}

func DeveloperMessage(content string) Message {
	return oa.DeveloperMessage(content)
}

func UserMessage(content string) Message {
	return oa.UserMessage(content)
}

func ToolMessage(content, toolCallID string) Message {
	return oa.ToolMessage(content, toolCallID)
}

func AssistantMessage(content string, calls []ToolCall) Message {
	msg := oa.ChatCompletionAssistantMessageParam{}
	if content != "" {
		msg.Content.OfString = oa.String(content)
	}
	if len(calls) > 0 {
		msg.ToolCalls = make([]oa.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
		for _, call := range calls {
			msg.ToolCalls = append(msg.ToolCalls, oa.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &oa.ChatCompletionMessageFunctionToolCallParam{
					ID: call.ID,
					Function: oa.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      ToolCallName(call),
						Arguments: ToolCallArguments(call),
					},
				},
			})
		}
	}
	return Message{OfAssistant: &msg}
}

func FunctionToolCall(id, name, argumentsJSON string) ToolCall {
	return ToolCall{
		ID:   id,
		Type: "function",
		Function: oa.ChatCompletionMessageFunctionToolCallFunction{
			Name:      name,
			Arguments: argumentsJSON,
		},
	}
}

func ToolCallID(call ToolCall) string {
	return call.ID
}

func ToolCallName(call ToolCall) string {
	return call.Function.Name
}

func ToolCallArguments(call ToolCall) string {
	return call.Function.Arguments
}

func MessageToolCallID(msg Message) string {
	id := msg.GetToolCallID()
	if id == nil {
		return ""
	}
	return *id
}

func MessageRole(msg Message) string {
	switch {
	case msg.OfDeveloper != nil:
		return "developer"
	case msg.OfSystem != nil:
		return "system"
	case msg.OfUser != nil:
		return "user"
	case msg.OfAssistant != nil:
		return "assistant"
	case msg.OfTool != nil:
		return "tool"
	case msg.OfFunction != nil:
		return "function"
	}
	role := msg.GetRole()
	if role == nil {
		return ""
	}
	return *role
}

func MessageContent(msg Message) string {
	if msg.OfDeveloper != nil {
		return msg.OfDeveloper.Content.OfString.Or("")
	}
	if msg.OfAssistant != nil {
		return msg.OfAssistant.Content.OfString.Or("")
	}
	content := msg.GetContent().AsAny()
	text, ok := content.(*string)
	if !ok || text == nil {
		return ""
	}
	return *text
}

func MessageToolCalls(msg Message) []ToolCall {
	if msg.OfAssistant != nil && len(msg.OfAssistant.ToolCalls) > 0 {
		return toolCallsFromParams(msg.OfAssistant.ToolCalls)
	}
	return toolCallsFromParams(msg.GetToolCalls())
}

func toolCallsFromParams(params []oa.ChatCompletionMessageToolCallUnionParam) []ToolCall {
	if len(params) == 0 {
		return nil
	}
	calls := make([]ToolCall, 0, len(params))
	for _, param := range params {
		fn := param.GetFunction()
		id := param.GetID()
		if fn == nil || id == nil {
			continue
		}
		calls = append(calls, FunctionToolCall(*id, fn.Name, fn.Arguments))
	}
	return calls
}
