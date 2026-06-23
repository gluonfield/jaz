package browserworker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

type recordingBackend struct {
	input ActionInput
	out   ActionOutput
}

type scriptedBackend struct {
	calls []ActionInput
}

func (b *recordingBackend) Call(_ context.Context, input ActionInput) (ActionOutput, error) {
	b.input = input
	return b.out, nil
}

func (b *scriptedBackend) Call(_ context.Context, input ActionInput) (ActionOutput, error) {
	b.calls = append(b.calls, input)
	switch input.Action {
	case "state":
		data, _ := json.Marshal(PageState{
			URL:        "https://example.com",
			Title:      "Example",
			ReadyState: "complete",
			Text:       "Sign in to continue",
			Elements: []StateElement{{
				Ref:  "e1",
				Tag:  "button",
				Role: "button",
				Name: "Sign in",
			}},
		})
		return ActionOutput{Status: "ok", Data: data}, nil
	case "click":
		return ActionOutput{Status: "ok", Text: "Clicked."}, nil
	case "screenshot":
		return ActionOutput{Status: "ok", ImageBase64: "cG5n", ImageMIMEType: "image/png"}, nil
	default:
		return ActionOutput{Status: "ok", Text: input.Action}, nil
	}
}

func TestUnavailableBackendReportsMissingBridge(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "browserworker", Version: "test"}, nil)
	AddMCPTools(server, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      ToolName,
		Arguments: map[string]any{"action": "status"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("result = %#v, want error result", result)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "browser backend is not connected") {
		t.Fatalf("content = %#v", result.Content)
	}
}

func TestToolPassesSessionAndImageContent(t *testing.T) {
	backend := &recordingBackend{out: ActionOutput{
		Status:        "ok",
		Text:          "captured",
		ImageBase64:   "aW1hZ2U=",
		ImageMIMEType: "image/png",
	}}
	result, _, err := (tool{backend: backend}).Call(context.Background(), &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: mcpsession.Header("browser-session-1")},
	}, ActionInput{Action: "screenshot", Amount: 120})
	if err != nil {
		t.Fatal(err)
	}
	if backend.input.Session != "browser-session-1" || backend.input.Amount != 120 {
		t.Fatalf("input = %#v", backend.input)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %#v", result.Content)
	}
	if text, ok := result.Content[0].(*mcp.TextContent); !ok || text.Text != "captured" {
		t.Fatalf("text content = %#v", result.Content[0])
	}
	if image, ok := result.Content[1].(*mcp.ImageContent); !ok || image.MIMEType != "image/png" || string(image.Data) != "aW1hZ2U=" {
		t.Fatalf("image content = %#v", result.Content[1])
	}
}

func TestContentResultEmbedsPDFResource(t *testing.T) {
	result := contentResult(ActionOutput{
		Text:      "PDF captured.",
		PDFBase64: "JVBERg==",
	})
	if len(result.Content) != 2 {
		t.Fatalf("content = %#v", result.Content)
	}
	resource, ok := result.Content[1].(*mcp.EmbeddedResource)
	if !ok {
		t.Fatalf("content[1] = %#v", result.Content[1])
	}
	if resource.Resource.URI != "jaz://browser/output.pdf" || resource.Resource.MIMEType != "application/pdf" || string(resource.Resource.Blob) != "%PDF" {
		t.Fatalf("resource = %#v", resource.Resource)
	}
}

func TestHighLevelDoUsesStateRefs(t *testing.T) {
	backend := &scriptedBackend{}
	_, out, err := (highLevelTools{executor: NewHighLevelExecutor(backend)}).Do(context.Background(), &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: mcpsession.Header("browser-session-1")},
	}, HighLevelInput{Task: "click sign in"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "Clicked.") || !strings.Contains(out.Text, "ref=e1") {
		t.Fatalf("out = %#v", out)
	}
	if len(backend.calls) < 2 || backend.calls[0].Action != "state" || backend.calls[1].Action != "click" || backend.calls[1].Selector != "ref=e1" {
		t.Fatalf("calls = %#v", backend.calls)
	}
}

func TestHighLevelGetAttachesVisualOnlyWhenRequested(t *testing.T) {
	backend := &scriptedBackend{}
	result, out, err := (highLevelTools{executor: NewHighLevelExecutor(backend)}).Get(context.Background(), &mcp.CallToolRequest{}, HighLevelInput{Visual: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "Targets:") || out.ImageBase64 != "cG5n" {
		t.Fatalf("out = %#v", out)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %#v", result.Content)
	}
	if len(backend.calls) != 2 || backend.calls[0].Action != "state" || backend.calls[1].Action != "screenshot" {
		t.Fatalf("calls = %#v", backend.calls)
	}
}
