package widget

import (
	"context"
	"testing"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/internal/widgets"
)

type fakePublisher struct {
	sessionID string
	input     widgets.PublishInput
}

func (p *fakePublisher) PublishForSession(sessionID string, input widgets.PublishInput) (widgets.Widget, []string, error) {
	p.sessionID = sessionID
	p.input = input
	return widgets.Widget{ID: "widget-1", Title: "Inbox", CurrentVersion: 2, SizeHint: "2x2"}, []string{"tight spacing"}, nil
}

func TestPublishToolPublishesForSession(t *testing.T) {
	publisher := &fakePublisher{}
	tool := PublishTool{Publisher: publisher}
	if got := tools.DefinitionName(tool.Definition()); got != widgets.PublishToolName {
		t.Fatalf("tool name = %q, want %q", got, widgets.PublishToolName)
	}
	result, err := tool.Execute(sessioncontext.WithSessionID(context.Background(), "session-1"), map[string]any{
		"title":     "Inbox",
		"size_hint": "2x2",
		"html":      "<section>ok</section>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if publisher.sessionID != "session-1" {
		t.Fatalf("session id = %q", publisher.sessionID)
	}
	if publisher.input.Title != "Inbox" || publisher.input.SizeHint != "2x2" || publisher.input.HTML == "" {
		t.Fatalf("input = %#v", publisher.input)
	}
	if result.Content == "" || result.Content == "{}" {
		t.Fatalf("result content = %q", result.Content)
	}
}
