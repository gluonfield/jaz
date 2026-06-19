package widgets

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

type fakeMCPPublisher struct {
	sessionID string
	input     PublishInput
}

func (f *fakeMCPPublisher) PublishForSession(sessionID string, input PublishInput) (Widget, []string, error) {
	f.sessionID = sessionID
	f.input = input
	return Widget{ID: "widget-1", Title: "Inbox", CurrentVersion: 4, SizeHint: "2x2"}, []string{"tighten layout"}, nil
}

func TestMCPPublishUsesSessionHeader(t *testing.T) {
	publisher := &fakeMCPPublisher{}
	result, out, err := publish(publisher, &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: mcpsession.Header("thread-1")},
	}, MCPPublishInput{Title: "Inbox", SizeHint: "2x2", HTML: "<p>hi</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if publisher.sessionID != "thread-1" {
		t.Fatalf("session = %q", publisher.sessionID)
	}
	if publisher.input.Title != "Inbox" || publisher.input.SizeHint != "2x2" || publisher.input.HTML != "<p>hi</p>" {
		t.Fatalf("input = %#v", publisher.input)
	}
	if out.WidgetID != "widget-1" || out.Version != 4 || len(out.Warnings) != 1 {
		t.Fatalf("output = %#v", out)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %d", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "Published widget") || !strings.Contains(text.Text, "Host-rendering warnings") {
		t.Fatalf("content = %#v", result.Content[0])
	}
	if strings.Contains(text.Text, "Lint warnings") {
		t.Fatalf("publish output should not frame advisory host feedback as lint: %q", text.Text)
	}
}

func TestMCPPublishRequiresSessionHeader(t *testing.T) {
	_, _, err := publish(&fakeMCPPublisher{}, &mcp.CallToolRequest{}, MCPPublishInput{})
	if err == nil || !strings.Contains(err.Error(), mcpsession.HeaderName) {
		t.Fatalf("err = %v", err)
	}
}
