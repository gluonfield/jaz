package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	oa "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/wins/jaz/backend/internal/provider"
)

type ChatMessage = oa.ChatCompletionMessageParamUnion
type ChatTool = oa.ChatCompletionToolUnionParam
type FunctionDefinition = shared.FunctionDefinitionParam

type Provider struct {
	BaseURL      string
	APIKey       string
	Model        string
	Client       *http.Client
	IncludeUsage bool
}

func New(baseURL, apiKey, model string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &Provider{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		APIKey:       apiKey,
		Model:        model,
		Client:       http.DefaultClient,
		IncludeUsage: strings.Contains(baseURL, "openrouter.ai"),
	}
}

func (p *Provider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, errors.New("model is required")
	}
	reasoningEffort, err := provider.NormalizeReasoningEffort(req.ReasoningEffort)
	if err != nil {
		return nil, err
	}

	client := p.client()
	params := oa.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: req.Messages,
		Tools:    req.Tools,
		StreamOptions: oa.ChatCompletionStreamOptionsParam{
			IncludeUsage: oa.Bool(true),
		},
	}
	if reasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(reasoningEffort)
	}
	stream := client.Chat.Completions.NewStreaming(ctx, params, p.requestOptions()...)

	events := make(chan provider.Event)
	go func() {
		defer close(events)
		acc := oa.ChatCompletionAccumulator{}
		var usage provider.Usage
		for stream.Next() {
			chunk := stream.Current()
			usage = mergeUsageSnapshot(usage, usageFromOpenAI(chunk.Usage))
			if !acc.AddChunk(chunk) {
				events <- provider.Event{Type: provider.EventError, Err: errors.New("failed to accumulate chat stream chunk")}
				return
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					events <- provider.Event{Type: provider.EventDelta, Delta: choice.Delta.Content}
				}
			}
			for _, delta := range reasoningDeltas(chunk.RawJSON()) {
				events <- provider.Event{Type: provider.EventReasoning, Reasoning: delta}
			}
		}
		if err := stream.Err(); err != nil {
			events <- provider.Event{Type: provider.EventError, Err: err}
			return
		}
		usage = mergeUsageSnapshot(usage, usageFromOpenAI(acc.Usage))
		emitToolCalls(acc, events)
		events <- provider.Event{Type: provider.EventDone, Usage: splitCachedInput(usage)}
	}()
	return events, nil
}

func (p *Provider) client() oa.Client {
	opts := []option.RequestOption{option.WithBaseURL(p.BaseURL)}
	if p.APIKey != "" {
		opts = append(opts, option.WithAPIKey(p.APIKey))
	}
	if p.Client != nil {
		opts = append(opts, option.WithHTTPClient(p.Client))
	}
	return oa.NewClient(opts...)
}

func (p *Provider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return provider.Response{}, errors.New("model is required")
	}
	reasoningEffort, err := provider.NormalizeReasoningEffort(req.ReasoningEffort)
	if err != nil {
		return provider.Response{}, err
	}

	client := p.client()
	params := oa.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: req.Messages,
		Tools:    req.Tools,
	}
	if reasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(reasoningEffort)
	}
	resp, err := client.Chat.Completions.New(ctx, params, p.requestOptions()...)
	if err != nil {
		return provider.Response{}, err
	}
	if len(resp.Choices) == 0 {
		return provider.Response{}, errors.New("provider returned no choices")
	}
	return provider.Response{Message: resp.Choices[0].Message.ToParam(), Usage: splitCachedInput(usageFromOpenAI(resp.Usage))}, nil
}

// usageFromOpenAI maps the wire shape verbatim — prompt_tokens still counts
// cached reads inclusively. splitCachedInput converts to the disjoint
// convention exactly once, at the response boundary; streaming merges
// partial snapshots in between, so splitting any earlier double-counts.
func usageFromOpenAI(usage oa.CompletionUsage) provider.Usage {
	return provider.Usage{
		InputTokens:           usage.PromptTokens,
		CachedInputTokens:     cachedInputTokens(usage),
		OutputTokens:          usage.CompletionTokens,
		ReasoningOutputTokens: usage.CompletionTokensDetails.ReasoningTokens,
		TotalTokens:           usage.TotalTokens,
	}
}

func splitCachedInput(u provider.Usage) provider.Usage {
	if u.CachedInputTokens > 0 && u.CachedInputTokens <= u.InputTokens {
		u.InputTokens -= u.CachedInputTokens
	}
	return u
}

func (p *Provider) requestOptions() []option.RequestOption {
	if !p.IncludeUsage {
		return nil
	}
	return []option.RequestOption{option.WithJSONSet("usage", map[string]any{"include": true})}
}

func mergeUsageSnapshot(current, next provider.Usage) provider.Usage {
	if next.InputTokens > 0 {
		current.InputTokens = next.InputTokens
	}
	if next.CachedInputTokens > 0 {
		current.CachedInputTokens = next.CachedInputTokens
	}
	if next.OutputTokens > 0 {
		current.OutputTokens = next.OutputTokens
	}
	if next.ReasoningOutputTokens > 0 {
		current.ReasoningOutputTokens = next.ReasoningOutputTokens
	}
	if next.TotalTokens > 0 {
		current.TotalTokens = next.TotalTokens
	}
	return current
}

func reasoningDeltas(raw string) []string {
	var chunk struct {
		Choices []struct {
			Delta map[string]json.RawMessage `json:"delta"`
		} `json:"choices"`
	}
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &chunk) != nil {
		return nil
	}
	var out []string
	for _, choice := range chunk.Choices {
		for _, key := range []string{"reasoning", "reasoning_content", "reasoning_delta", "thinking", "thought"} {
			out = append(out, textValues(choice.Delta[key])...)
		}
	}
	return out
}

func textValues(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s != "" {
			return []string{s}
		}
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range []string{"text", "delta", "content"} {
			if values := textValues(obj[key]); len(values) > 0 {
				return values
			}
		}
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		var out []string
		for _, item := range arr {
			out = append(out, textValues(item)...)
		}
		return out
	}
	return nil
}

func emitToolCalls(acc oa.ChatCompletionAccumulator, events chan<- provider.Event) {
	if len(acc.Choices) == 0 {
		return
	}
	for i, call := range acc.Choices[0].Message.ToolCalls {
		if call.Function.Name == "" {
			continue
		}
		id := call.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		events <- provider.Event{
			Type: provider.EventToolCall,
			ToolCall: &provider.ToolCall{
				ID:       id,
				Type:     call.Type,
				Function: call.Function,
			},
		}
	}
}
