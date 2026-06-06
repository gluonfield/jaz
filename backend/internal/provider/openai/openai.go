package openai

import (
	"context"
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
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

func New(baseURL, apiKey, model string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &Provider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		Client:  http.DefaultClient,
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

	client := p.client()
	stream := client.Chat.Completions.NewStreaming(ctx, oa.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: req.Messages,
		Tools:    req.Tools,
		StreamOptions: oa.ChatCompletionStreamOptionsParam{
			IncludeUsage: oa.Bool(true),
		},
	})

	events := make(chan provider.Event)
	go func() {
		defer close(events)
		acc := oa.ChatCompletionAccumulator{}
		for stream.Next() {
			chunk := stream.Current()
			if !acc.AddChunk(chunk) {
				events <- provider.Event{Type: provider.EventError, Err: errors.New("failed to accumulate chat stream chunk")}
				return
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					events <- provider.Event{Type: provider.EventDelta, Delta: choice.Delta.Content}
				}
			}
		}
		if err := stream.Err(); err != nil {
			events <- provider.Event{Type: provider.EventError, Err: err}
			return
		}
		emitToolCalls(acc, events)
		events <- provider.Event{Type: provider.EventDone, Usage: usageFromOpenAI(acc.Usage)}
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

	client := p.client()
	resp, err := client.Chat.Completions.New(ctx, oa.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: req.Messages,
		Tools:    req.Tools,
	})
	if err != nil {
		return provider.Response{}, err
	}
	if len(resp.Choices) == 0 {
		return provider.Response{}, errors.New("provider returned no choices")
	}
	return provider.Response{Message: resp.Choices[0].Message.ToParam(), Usage: usageFromOpenAI(resp.Usage)}, nil
}

func usageFromOpenAI(usage oa.CompletionUsage) provider.Usage {
	return provider.Usage{
		InputTokens:           usage.PromptTokens,
		CachedInputTokens:     usage.PromptTokensDetails.CachedTokens,
		OutputTokens:          usage.CompletionTokens,
		ReasoningOutputTokens: usage.CompletionTokensDetails.ReasoningTokens,
		TotalTokens:           usage.TotalTokens,
	}
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
