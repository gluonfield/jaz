package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/tools"
)

type fakeProvider struct {
	calls    int
	requests []provider.Request
}

func (p *fakeProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	p.calls++
	p.requests = append(p.requests, req)
	if p.calls == 1 {
		return provider.Response{Message: provider.AssistantMessage("", []provider.ToolCall{
			provider.FunctionToolCall("call_1", "mock", `{"value":"ok"}`),
		})}, nil
	}
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *fakeProvider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.calls++
	p.requests = append(p.requests, req)
	ch := make(chan provider.Event, 4)
	go func() {
		defer close(ch)
		if p.calls == 1 {
			call := provider.FunctionToolCall("call_1", "mock", `{"value":"ok"}`)
			ch <- provider.Event{Type: provider.EventToolCall, ToolCall: &call}
			ch <- provider.Event{Type: provider.EventDone}
			return
		}
		ch <- provider.Event{Type: provider.EventDelta, Delta: "done"}
		ch <- provider.Event{Type: provider.EventDone}
	}()
	return ch, nil
}

type mockTool struct{}

func (mockTool) Definition() tools.Definition {
	return tools.Function("mock", "mock tool", false, map[string]any{"type": "object"})
}

func (mockTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	return tools.Result{Content: `{"status":"completed","value":"` + inputs["value"].(string) + `"}`}, nil
}

type namedTool struct {
	name        string
	description string
	parameters  map[string]any
}

func (t namedTool) Definition() tools.Definition {
	return tools.Function(t.name, t.description, false, t.parameters)
}

func (t namedTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	return tools.Result{Content: `{"status":"completed","tool":"` + t.name + `"}`}, nil
}

func TestAgentToolLoop(t *testing.T) {
	a := &Agent{
		Provider: &fakeProvider{},
		Tools:    tools.NewRegistry(mockTool{}),
	}

	var text strings.Builder
	var sawTool bool
	messages := []provider.Message{provider.UserMessage("hello")}
	for event := range a.Run(context.Background(), provider.Request{Messages: messages}) {
		if event.Type == StreamToolResult {
			sawTool = true
		}
		if event.Type == StreamDelta {
			text.WriteString(event.Delta)
		}
	}
	if !sawTool {
		t.Fatal("expected tool result event")
	}
	if text.String() != "done" {
		t.Fatalf("unexpected streamed text %q", text.String())
	}
}

func TestAgentCompleteReturnsFinalResult(t *testing.T) {
	a := &Agent{
		Provider: &fakeProvider{},
		Tools:    tools.NewRegistry(mockTool{}),
	}
	result, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{provider.UserMessage("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "done" {
		t.Fatalf("unexpected content %q", result.Content)
	}
	if len(result.ToolExecutions) != 1 || provider.ToolCallName(result.ToolExecutions[0].Call) != "mock" {
		t.Fatalf("unexpected tool executions %#v", result.ToolExecutions)
	}
	if len(result.Messages) != 4 {
		t.Fatalf("expected user, assistant-call, tool, assistant messages; got %d", len(result.Messages))
	}
}

type toolSearchProvider struct {
	requests []provider.Request
}

func (p *toolSearchProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	p.requests = append(p.requests, req)
	switch len(p.requests) {
	case 1:
		return provider.Response{Message: provider.AssistantMessage("", []provider.ToolCall{
			provider.FunctionToolCall("search_1", toolSearchToolName, `{"query":"calendar event","limit":1}`),
		})}, nil
	case 2:
		return provider.Response{Message: provider.AssistantMessage("", []provider.ToolCall{
			provider.FunctionToolCall("mcp_1", "mcp_calendar_create_event", `{"title":"Lunch"}`),
		})}, nil
	default:
		return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
	}
}

func (p *toolSearchProvider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	panic("not used")
}

func TestAgentDefersMCPToolsBehindToolSearch(t *testing.T) {
	fp := &toolSearchProvider{}
	a := &Agent{
		Provider: fp,
		Tools: tools.NewRegistry(
			mockTool{},
			namedTool{
				name:        "mcp_calendar_create_event",
				description: "Create calendar events.",
				parameters: tools.ObjectSchema(map[string]any{
					"title": tools.StringSchema("Event title."),
				}, []string{"title"}),
			},
		),
		DeferTools: func(name string) bool { return strings.HasPrefix(name, "mcp_") },
	}

	result, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{provider.UserMessage("create calendar event")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "done" {
		t.Fatalf("content = %q, want done", result.Content)
	}
	if len(fp.requests) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(fp.requests))
	}

	firstTools := requestToolNames(fp.requests[0])
	if !contains(firstTools, toolSearchToolName) {
		t.Fatalf("first request tools = %v, want %s", firstTools, toolSearchToolName)
	}
	if contains(firstTools, "mcp_calendar_create_event") {
		t.Fatalf("first request leaked deferred MCP tool: %v", firstTools)
	}
	if !contains(firstTools, "mock") {
		t.Fatalf("first request lost direct tool: %v", firstTools)
	}

	var searchOutput struct {
		Status    string           `json:"status"`
		Execution string           `json:"execution"`
		Tools     []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal([]byte(result.ToolExecutions[0].Result), &searchOutput); err != nil {
		t.Fatal(err)
	}
	if searchOutput.Status != "completed" || searchOutput.Execution != "client" || len(searchOutput.Tools) != 1 {
		t.Fatalf("unexpected tool_search output: %#v", searchOutput)
	}
	if searchOutput.Tools[0]["name"] != "mcp_calendar_create_event" || searchOutput.Tools[0]["defer_loading"] != true {
		t.Fatalf("unexpected returned tool spec: %#v", searchOutput.Tools[0])
	}

	secondTools := requestToolNames(fp.requests[1])
	if !contains(secondTools, "mcp_calendar_create_event") {
		t.Fatalf("second request tools = %v, want searched MCP tool", secondTools)
	}
	if len(result.ToolExecutions) != 2 || provider.ToolCallName(result.ToolExecutions[1].Call) != "mcp_calendar_create_event" {
		t.Fatalf("unexpected tool executions: %#v", result.ToolExecutions)
	}
}

type sameBatchSearchProvider struct {
	requests []provider.Request
}

func (p *sameBatchSearchProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	p.requests = append(p.requests, req)
	if len(p.requests) == 1 {
		return provider.Response{Message: provider.AssistantMessage("", []provider.ToolCall{
			provider.FunctionToolCall("search_1", toolSearchToolName, `{"query":"calendar event","limit":1}`),
			provider.FunctionToolCall("mcp_1", "mcp_calendar_create_event", `{"title":"Lunch"}`),
		})}, nil
	}
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *sameBatchSearchProvider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	panic("not used")
}

func TestToolSearchExposesMatchesOnNextModelCallOnly(t *testing.T) {
	fp := &sameBatchSearchProvider{}
	a := &Agent{
		Provider: fp,
		Tools: tools.NewRegistry(namedTool{
			name:        "mcp_calendar_create_event",
			description: "Create calendar events.",
			parameters:  tools.ObjectSchema(map[string]any{"title": tools.StringSchema("Event title.")}, []string{"title"}),
		}),
		DeferTools: func(name string) bool { return strings.HasPrefix(name, "mcp_") },
	}

	result, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{provider.UserMessage("create calendar event")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ToolExecutions) != 2 {
		t.Fatalf("tool executions = %d, want search and rejected mcp call", len(result.ToolExecutions))
	}
	if !strings.Contains(result.ToolExecutions[1].Result, "use tool_search first") {
		t.Fatalf("same-batch deferred tool was not rejected: %s", result.ToolExecutions[1].Result)
	}
	secondTools := requestToolNames(fp.requests[1])
	if !contains(secondTools, "mcp_calendar_create_event") {
		t.Fatalf("next request tools = %v, want searched MCP tool", secondTools)
	}
}

func TestToolSearchMatchesSchemaTerms(t *testing.T) {
	exposure, err := newToolExposure([]tools.Definition{
		namedTool{
			name:        "mcp_calendar_create_event",
			description: "Create events.",
			parameters: tools.ObjectSchema(map[string]any{
				"title": tools.StringSchema("Event title."),
			}, []string{"title"}),
		}.Definition(),
		namedTool{
			name:        "mcp_invoice_lookup",
			description: "Fetch records.",
			parameters: tools.ObjectSchema(map[string]any{
				"ledger_id": tools.StringSchema("Reconcile ledger entries."),
			}, []string{"ledger_id"}),
		}.Definition(),
	}, nil, func(name string) bool { return strings.HasPrefix(name, "mcp_") })
	if err != nil {
		t.Fatal(err)
	}

	result := exposure.executeSearch(provider.FunctionToolCall("search_1", toolSearchToolName, `{"query":"reconcile ledger","limit":1}`))
	var output struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Tools) != 1 || output.Tools[0]["name"] != "mcp_invoice_lookup" {
		t.Fatalf("schema search returned %#v", output.Tools)
	}
}

func TestToolSearchTokenizesLikeCodexEnglishBM25(t *testing.T) {
	if got := tokenizeToolSearch("i me my myself we our ours ourselves you you're you've you'll you'd"); len(got) != 0 {
		t.Fatalf("stopword tokens = %v, want none", got)
	}
	if got := tokenizeToolSearch("space,station 42 1337 3.14"); !reflect.DeepEqual(got, []string{"space", "station", "42", "1337", "3.14"}) {
		t.Fatalf("punctuation/number tokens = %v", got)
	}
	if got := tokenizeToolSearch("connection connections connective connected connecting connect"); !reflect.DeepEqual(got, []string{"connect", "connect", "connect", "connect", "connect", "connect"}) {
		t.Fatalf("stemmed tokens = %v", got)
	}
}

func TestToolSearchStemsEnglishTerms(t *testing.T) {
	exposure, err := newToolExposure([]tools.Definition{
		namedTool{
			name:        "mcp_invoice_lookup",
			description: "Fetch records.",
			parameters: tools.ObjectSchema(map[string]any{
				"connection_id": tools.StringSchema("Payment connection identifier."),
			}, []string{"connection_id"}),
		}.Definition(),
	}, nil, func(name string) bool { return strings.HasPrefix(name, "mcp_") })
	if err != nil {
		t.Fatal(err)
	}

	result := exposure.executeSearch(provider.FunctionToolCall("search_1", toolSearchToolName, `{"query":"connected payment","limit":1}`))
	var output struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Tools) != 1 || output.Tools[0]["name"] != "mcp_invoice_lookup" {
		t.Fatalf("stemmed search returned %#v", output.Tools)
	}
}

func TestToolSearchRestoresExposedToolsFromHistory(t *testing.T) {
	fp := &resumedToolSearchProvider{}
	a := &Agent{
		Provider: fp,
		Tools: tools.NewRegistry(namedTool{
			name:        "mcp_calendar_create_event",
			description: "Create calendar events.",
			parameters:  tools.ObjectSchema(map[string]any{"title": tools.StringSchema("Event title.")}, []string{"title"}),
		}),
		DeferTools: func(name string) bool { return strings.HasPrefix(name, "mcp_") },
	}

	_, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{
			provider.UserMessage("find the calendar tool"),
			provider.AssistantMessage("", []provider.ToolCall{
				provider.FunctionToolCall("search_1", toolSearchToolName, `{"query":"calendar event","limit":1}`),
			}),
			provider.ToolMessage(`{"status":"completed","execution":"client","tools":[{"type":"function","name":"mcp_calendar_create_event","strict":false,"defer_loading":true}]}`, "search_1"),
			provider.UserMessage("use the calendar tool"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstTools := requestToolNames(fp.requests[0])
	if !contains(firstTools, "mcp_calendar_create_event") {
		t.Fatalf("restored request tools = %v, want prior searched MCP tool", firstTools)
	}
	if contains(firstTools, "mcp_unrelated_tool") {
		t.Fatalf("restored unrelated tool: %v", firstTools)
	}
}

type resumedToolSearchProvider struct {
	requests []provider.Request
}

func (p *resumedToolSearchProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	p.requests = append(p.requests, req)
	if len(p.requests) == 1 {
		return provider.Response{Message: provider.AssistantMessage("", []provider.ToolCall{
			provider.FunctionToolCall("mcp_1", "mcp_calendar_create_event", `{"title":"Lunch"}`),
		})}, nil
	}
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *resumedToolSearchProvider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	panic("not used")
}

func requestToolNames(req provider.Request) []string {
	names := make([]string, 0, len(req.Tools))
	for _, def := range req.Tools {
		names = append(names, tools.DefinitionName(def))
	}
	return names
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestAddUsageAccumulatesCacheWrites(t *testing.T) {
	usage := addUsage(provider.Usage{
		InputTokens:           10,
		CachedInputTokens:     20,
		CachedWriteTokens:     30,
		OutputTokens:          40,
		ReasoningOutputTokens: 5,
		TotalTokens:           100,
	}, provider.Usage{
		InputTokens:           1,
		CachedInputTokens:     2,
		CachedWriteTokens:     3,
		OutputTokens:          4,
		ReasoningOutputTokens: 6,
		TotalTokens:           15,
	})
	if usage.InputTokens != 11 || usage.CachedInputTokens != 22 || usage.CachedWriteTokens != 33 ||
		usage.OutputTokens != 44 || usage.ReasoningOutputTokens != 11 || usage.TotalTokens != 115 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestAgentUsesDefaultReasoningEffort(t *testing.T) {
	fp := &fakeProvider{}
	a := &Agent{
		Provider:        fp,
		ModelProvider:   "openrouter",
		ReasoningEffort: "high",
		Tools:           tools.NewRegistry(mockTool{}),
	}
	_, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{provider.UserMessage("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.requests) == 0 || fp.requests[0].Provider != "openrouter" || fp.requests[0].ReasoningEffort != "high" {
		t.Fatalf("provider defaults were not forwarded: %#v", fp.requests)
	}
}

type mediaProvider struct {
	calls    int
	requests []provider.Request
}

func (p *mediaProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	p.calls++
	p.requests = append(p.requests, req)
	if p.calls == 1 {
		return provider.Response{Message: provider.AssistantMessage("", []provider.ToolCall{
			provider.FunctionToolCall("call_media", "media_mock", `{}`),
		})}, nil
	}
	return provider.Response{Message: provider.AssistantMessage("saw it", nil)}, nil
}

func (p *mediaProvider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	panic("not used")
}

type mediaRefTool struct {
	blobPath string
	sha      string
	size     int64
}

func (t mediaRefTool) Definition() tools.Definition {
	return tools.Function("media_mock", "media mock", false, map[string]any{"type": "object"})
}

func (t mediaRefTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	content, err := json.Marshal(map[string]any{
		"status":  "ok",
		"message": "Image attached for visual inspection.",
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Content: string(content),
		MediaRefs: []media.Ref{{
			Type:     media.TypeInputImage,
			Text:     "Image returned by view_image: image.png",
			BlobPath: t.blobPath,
			MimeType: "image/png",
			Size:     t.size,
			SHA256:   t.sha,
			Detail:   "auto",
			Filename: "image.png",
		}},
	}, nil
}

func TestAgentMaterializesToolMediaRefsWithoutPersistingBase64(t *testing.T) {
	blobPath, sha, size := writeMediaBlob(t)
	fp := &mediaProvider{}
	a := &Agent{
		Provider: fp,
		Tools:    tools.NewRegistry(mediaRefTool{blobPath: blobPath, sha: sha, size: size}),
	}

	result, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{provider.UserMessage("inspect image")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "saw it" {
		t.Fatalf("content = %q, want final response", result.Content)
	}
	if len(fp.requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(fp.requests))
	}
	if len(fp.requests[1].Messages) != 4 {
		t.Fatalf("second request messages = %d, want durable user+assistant+tool plus synthetic image", len(fp.requests[1].Messages))
	}
	requestJSON, err := json.Marshal(fp.requests[1].Messages)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(requestJSON), "data:image/png;base64,") {
		t.Fatalf("second request did not contain materialized image data: %s", requestJSON)
	}
	if provider.MessageContent(fp.requests[1].Messages[3]) != "Image returned by view_image: image.png" {
		t.Fatalf("synthetic image text = %q", provider.MessageContent(fp.requests[1].Messages[3]))
	}
	if len(result.Messages) != 4 {
		t.Fatalf("durable messages = %d, want user, assistant tool-call, tool result, assistant", len(result.Messages))
	}
	durableJSON, err := json.Marshal(result.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(durableJSON), "data:image") {
		t.Fatalf("durable messages leaked base64 image data: %s", durableJSON)
	}
}

func TestAgentFailsClosedWhenMediaBlobCannotBeReplayed(t *testing.T) {
	_, sha, size := writeMediaBlob(t)
	fp := &mediaProvider{}
	a := &Agent{
		Provider: fp,
		Tools:    tools.NewRegistry(mediaRefTool{blobPath: filepath.Join(t.TempDir(), "missing"), sha: sha, size: size}),
	}

	result, err := a.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{provider.UserMessage("inspect image")},
	})
	if err == nil {
		t.Fatal("expected missing media blob to stop the loop")
	}
	if len(fp.requests) != 1 {
		t.Fatalf("provider calls = %d, want no second request after failed media replay", len(fp.requests))
	}
	if len(result.Messages) != 3 {
		t.Fatalf("durable messages = %d, want state through tool result", len(result.Messages))
	}
}

func writeMediaBlob(t *testing.T) (string, string, int64) {
	t.Helper()
	data := []byte("\x89PNG\r\n\x1a\nimage-bytes")
	sum := sha256.Sum256(data)
	dir := t.TempDir()
	path := filepath.Join(dir, "image-blob")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, hex.EncodeToString(sum[:]), int64(len(data))
}
