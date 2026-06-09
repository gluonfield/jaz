package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
	tooldefs "github.com/wins/jaz/backend/internal/tools"
)

const (
	defaultBaseURL   = "https://api.anthropic.com/v1"
	defaultMaxTokens = 8192
	apiVersion       = "2023-06-01"
)

type Provider struct {
	BaseURL   string
	APIKey    string
	Model     string
	Client    *http.Client
	MaxTokens int
}

func New(baseURL, apiKey, model string) *Provider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Provider{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		APIKey:    apiKey,
		Model:     model,
		Client:    http.DefaultClient,
		MaxTokens: defaultMaxTokens,
	}
}

func (p *Provider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	body, err := p.buildRequest(req, false)
	if err != nil {
		return provider.Response{}, err
	}
	resp, err := p.post(ctx, body)
	if err != nil {
		return provider.Response{}, err
	}
	defer resp.Body.Close()
	if err := anthropicHTTPError(resp); err != nil {
		return provider.Response{}, err
	}

	var data messageResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return provider.Response{}, err
	}
	message := responseMessage(data.Content)
	if provider.MessageContent(message) == "" && len(provider.MessageToolCalls(message)) == 0 {
		return provider.Response{}, errors.New("provider returned no content")
	}
	return provider.Response{Message: message, Usage: usageFromAnthropic(data.Usage)}, nil
}

func (p *Provider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	body, err := p.buildRequest(req, true)
	if err != nil {
		return nil, err
	}
	resp, err := p.post(ctx, body)
	if err != nil {
		return nil, err
	}
	if err := anthropicHTTPError(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	events := make(chan provider.Event)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		parseStream(resp.Body, events)
	}()
	return events, nil
}

func (p *Provider) buildRequest(req provider.Request, stream bool) (messageRequest, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(p.Model)
	}
	if model == "" {
		return messageRequest{}, errors.New("model is required")
	}
	reasoningEffort, err := provider.NormalizeReasoningEffort(req.ReasoningEffort)
	if err != nil {
		return messageRequest{}, err
	}
	system, messages := convertMessages(req.Messages)
	thinking := thinkingForEffort(reasoningEffort)
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	if thinking != nil && maxTokens <= thinking.BudgetTokens {
		maxTokens = thinking.BudgetTokens + 1024
	}
	return messageRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  messages,
		Tools:     convertTools(req.Tools),
		Thinking:  thinking,
		Stream:    stream,
	}, nil
}

func (p *Provider) post(ctx context.Context, body messageRequest) (*http.Response, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, errors.New("api key is required")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Api-Key", p.APIKey)
	httpReq.Header.Set("Anthropic-Version", apiVersion)
	return p.client().Do(httpReq)
}

func (p *Provider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

type messageRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Thinking  *thinking          `json:"thinking,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

type thinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type anthropicMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   any             `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type messageResponse struct {
	Content []contentBlock `json:"content"`
	Usage   anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
}

func convertMessages(messages []provider.Message) (string, []anthropicMessage) {
	var system []string
	var out []anthropicMessage
	for _, msg := range messages {
		role := provider.MessageRole(msg)
		content := provider.MessageContent(msg)
		switch role {
		case "developer", "system":
			if strings.TrimSpace(content) != "" {
				system = append(system, content)
			}
		case "assistant":
			blocks := textBlocks(content)
			for _, call := range provider.MessageToolCalls(msg) {
				blocks = append(blocks, contentBlock{
					Type:  "tool_use",
					ID:    provider.ToolCallID(call),
					Name:  provider.ToolCallName(call),
					Input: toolInput(provider.ToolCallArguments(call)),
				})
			}
			appendMessage(&out, "assistant", blocks)
		case "tool":
			toolUseID := provider.MessageToolCallID(msg)
			if strings.TrimSpace(toolUseID) == "" {
				continue
			}
			if content == "" {
				content = "{}"
			}
			appendMessage(&out, "user", []contentBlock{{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   content,
			}})
		default:
			appendMessage(&out, "user", textBlocks(content))
		}
	}
	return strings.Join(system, "\n\n"), out
}

func appendMessage(messages *[]anthropicMessage, role string, blocks []contentBlock) {
	if len(blocks) == 0 {
		return
	}
	last := len(*messages) - 1
	if last >= 0 && (*messages)[last].Role == role {
		(*messages)[last].Content = append((*messages)[last].Content, blocks...)
		return
	}
	*messages = append(*messages, anthropicMessage{Role: role, Content: blocks})
}

func textBlocks(text string) []contentBlock {
	if text == "" {
		return nil
	}
	return []contentBlock{{Type: "text", Text: text}}
}

func convertTools(defs []tooldefs.Definition) []anthropicTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]anthropicTool, 0, len(defs))
	for _, def := range defs {
		fn := def.GetFunction()
		if fn == nil || strings.TrimSpace(fn.Name) == "" {
			continue
		}
		schema := map[string]any(fn.Parameters)
		if len(schema) == 0 {
			schema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		out = append(out, anthropicTool{
			Name:        fn.Name,
			Description: fn.Description.Or(""),
			InputSchema: schema,
		})
	}
	return out
}

func thinkingForEffort(effort string) *thinking {
	switch effort {
	case "", "none":
		return nil
	case "minimal", "low":
		return &thinking{Type: "enabled", BudgetTokens: 1024}
	case "medium":
		return &thinking{Type: "enabled", BudgetTokens: 2048}
	case "high":
		return &thinking{Type: "enabled", BudgetTokens: 4096}
	case "xhigh":
		return &thinking{Type: "enabled", BudgetTokens: 8192}
	default:
		return nil
	}
}

func toolInput(arguments string) json.RawMessage {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return json.RawMessage(`{}`)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(arguments), &obj); err != nil || obj == nil {
		return json.RawMessage(`{}`)
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

func responseMessage(blocks []contentBlock) provider.Message {
	var text strings.Builder
	var calls []provider.ToolCall
	for _, block := range blocks {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			calls = append(calls, provider.FunctionToolCall(block.ID, block.Name, string(toolInput(string(block.Input)))))
		}
	}
	return provider.AssistantMessage(text.String(), calls)
}

func usageFromAnthropic(usage anthropicUsage) provider.Usage {
	total := usage.InputTokens + usage.OutputTokens
	return provider.Usage{
		InputTokens:       usage.InputTokens,
		CachedInputTokens: usage.CacheReadInputTokens,
		OutputTokens:      usage.OutputTokens,
		TotalTokens:       total,
	}
}

type streamEvent struct {
	Type         string         `json:"type"`
	Index        int            `json:"index"`
	ContentBlock contentBlock   `json:"content_block"`
	Delta        streamDelta    `json:"delta"`
	Message      streamMessage  `json:"message"`
	Usage        anthropicUsage `json:"usage"`
	Error        streamError    `json:"error"`
}

type streamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
}

type streamMessage struct {
	Usage anthropicUsage `json:"usage"`
}

type streamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type toolUseState struct {
	id        string
	name      string
	input     json.RawMessage
	inputJSON strings.Builder
}

func parseStream(body io.Reader, events chan<- provider.Event) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	var data strings.Builder
	var usage provider.Usage
	var done bool
	tools := map[int]*toolUseState{}

	flush := func() {
		raw := strings.TrimSpace(data.String())
		data.Reset()
		if raw == "" {
			return
		}
		eventUsage, eventDone := handleStreamEvent(raw, tools, events)
		usage = mergeUsage(usage, eventUsage)
		if eventDone {
			events <- provider.Event{Type: provider.EventDone, Usage: usage}
			done = true
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(value))
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		events <- provider.Event{Type: provider.EventError, Err: err}
		return
	}
	if !done {
		events <- provider.Event{Type: provider.EventDone, Usage: usage}
	}
}

func handleStreamEvent(raw string, tools map[int]*toolUseState, events chan<- provider.Event) (provider.Usage, bool) {
	var event streamEvent
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		events <- provider.Event{Type: provider.EventError, Err: err}
		return provider.Usage{}, false
	}
	switch event.Type {
	case "message_start":
		return usageFromAnthropic(event.Message.Usage), false
	case "content_block_start":
		switch event.ContentBlock.Type {
		case "text":
			if event.ContentBlock.Text != "" {
				events <- provider.Event{Type: provider.EventDelta, Delta: event.ContentBlock.Text}
			}
		case "thinking":
			thinking := event.ContentBlock.Thinking
			if thinking == "" {
				thinking = event.ContentBlock.Text
			}
			if thinking != "" {
				events <- provider.Event{Type: provider.EventReasoning, Reasoning: thinking}
			}
		case "tool_use":
			tools[event.Index] = &toolUseState{
				id:    event.ContentBlock.ID,
				name:  event.ContentBlock.Name,
				input: event.ContentBlock.Input,
			}
		}
	case "content_block_delta":
		switch event.Delta.Type {
		case "text_delta":
			if event.Delta.Text != "" {
				events <- provider.Event{Type: provider.EventDelta, Delta: event.Delta.Text}
			}
		case "thinking_delta":
			if event.Delta.Thinking != "" {
				events <- provider.Event{Type: provider.EventReasoning, Reasoning: event.Delta.Thinking}
			}
		case "input_json_delta":
			state := tools[event.Index]
			if state != nil {
				state.inputJSON.WriteString(event.Delta.PartialJSON)
			}
		}
	case "content_block_stop":
		state := tools[event.Index]
		if state != nil {
			input := state.input
			if state.inputJSON.Len() > 0 {
				input = json.RawMessage(state.inputJSON.String())
			}
			call := provider.FunctionToolCall(state.id, state.name, string(toolInput(string(input))))
			events <- provider.Event{Type: provider.EventToolCall, ToolCall: &call}
			delete(tools, event.Index)
		}
	case "message_delta":
		return usageFromAnthropic(event.Usage), false
	case "message_stop":
		return provider.Usage{}, true
	case "error":
		msg := event.Error.Message
		if msg == "" {
			msg = event.Error.Type
		}
		events <- provider.Event{Type: provider.EventError, Err: errors.New(msg)}
	}
	return provider.Usage{}, false
}

func mergeUsage(current, next provider.Usage) provider.Usage {
	if next.InputTokens > 0 {
		current.InputTokens = next.InputTokens
	}
	if next.CachedInputTokens > 0 {
		current.CachedInputTokens = next.CachedInputTokens
	}
	if next.OutputTokens > 0 {
		current.OutputTokens = next.OutputTokens
	}
	total := current.InputTokens + current.OutputTokens
	if total > 0 {
		current.TotalTokens = total
	} else if next.TotalTokens > current.TotalTokens {
		current.TotalTokens = next.TotalTokens
	}
	return current
}

func anthropicHTTPError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var data struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &data); err == nil && data.Error.Message != "" {
		return fmt.Errorf("anthropic error %s: %s", data.Error.Type, data.Error.Message)
	}
	return fmt.Errorf("anthropic error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
