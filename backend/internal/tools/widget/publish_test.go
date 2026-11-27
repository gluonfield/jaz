package widget

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/internal/widgets"
)

type fakePublisher struct {
	sessionID string
	input     widgets.PublishInput
}

func (p *fakePublisher) PublishForSession(_ context.Context, sessionID string, input widgets.PublishInput) (widgets.Published, error) {
	p.sessionID = sessionID
	p.input = input
	return widgets.Published{
		App:      storage.App{ID: "app-1", Title: "Inbox", CurrentVersion: 2},
		SizeHint: "2x2",
		Warnings: []string{"tight spacing"},
	}, nil
}

func TestPublishToolPublishesForSession(t *testing.T) {
	publisher := &fakePublisher{}
	tool := PublishTool{Publisher: publisher}
	if got := tools.DefinitionName(tool.Definition()); got != widgets.PublishMCPToolName {
		t.Fatalf("tool name = %q, want %q", got, widgets.PublishMCPToolName)
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
	var out widgets.MCPPublishOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatal(err)
	}
	if out.AppID != "app-1" || out.Version != 2 || out.SizeHint != "2x2" || len(out.Warnings) != 1 {
		t.Fatalf("result = %#v", out)
	}
}
