package widgets

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

type fakeMCPPublisher struct {
	sessionID string
	input     PublishInput
}

func (f *fakeMCPPublisher) PublishForSession(_ context.Context, sessionID string, input PublishInput) (Published, error) {
	f.sessionID = sessionID
	f.input = input
	return Published{
		App:      storage.App{ID: "app-1", Title: "Inbox", Kind: storage.AppKindArtifact, CurrentVersion: 4},
		SizeHint: "2x2",
		Warnings: []string{"tighten layout"},
	}, nil
}

func TestMCPPublishUsesSessionHeader(t *testing.T) {
	publisher := &fakeMCPPublisher{}
	out, err := publish(context.Background(), publisher, &mcp.CallToolRequest{
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
	if out.AppID != "app-1" || out.Version != 4 || out.SizeHint != "2x2" || len(out.Warnings) != 1 {
		t.Fatalf("output = %#v", out)
	}
	if !strings.Contains(out.Note, "warnings") {
		t.Fatalf("note should point at the warnings: %q", out.Note)
	}
}

func TestMCPPublishRequiresSessionHeader(t *testing.T) {
	_, err := publish(context.Background(), &fakeMCPPublisher{}, &mcp.CallToolRequest{}, MCPPublishInput{})
	if err == nil || !strings.Contains(err.Error(), mcpsession.HeaderName) {
		t.Fatalf("err = %v", err)
	}
}
