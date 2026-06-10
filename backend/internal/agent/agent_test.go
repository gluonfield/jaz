package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
