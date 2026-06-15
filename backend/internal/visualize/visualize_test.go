package visualize

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type fakeEventStore struct {
	events []sessionevents.Event
}

func (f *fakeEventStore) AppendSessionEvents(_ string, events ...sessionevents.Event) error {
	f.events = append(f.events, events...)
	return nil
}

func TestShowWidgetPublishesArtifactEventFromMCPHeader(t *testing.T) {
	store := &fakeEventStore{}
	tools := NewMCPTools(store, nil)
	result, structured, err := tools.ShowWidget(context.Background(), &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: mcpsession.Header("thread-1")},
	}, ShowWidgetInput{
		LoadingMessages: []string{"Rendering"},
		Title:           "System map",
		WidgetCode:      `<svg width="100%" viewBox="0 0 680 120" role="img"><title>Map</title><desc>System map</desc></svg>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if structured != nil {
		t.Fatalf("structured = %#v, want nil so the reminder text is not shadowed by structuredContent", structured)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %d, want 2", len(result.Content))
	}
	first, ok := result.Content[0].(*mcp.TextContent)
	if !ok || first.Text != RenderedMessage {
		t.Fatalf("first content = %#v", result.Content[0])
	}
	second, ok := result.Content[1].(*mcp.TextContent)
	if !ok || second.Text != RenderedReminder {
		t.Fatalf("second content = %#v", result.Content[1])
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want 1", len(store.events))
	}
	event := store.events[0]
	if event.SessionID != "thread-1" || event.Type != sessionevents.TypeArtifact {
		t.Fatalf("event = %#v", event)
	}
	if event.Artifact == nil || event.Artifact.Title != "System map" || event.Artifact.ArtifactType != "svg" {
		t.Fatalf("artifact = %#v", event.Artifact)
	}
}

func TestReadMeReturnsGuideAsContentWithoutStructured(t *testing.T) {
	tools := NewMCPTools(nil, nil)
	result, structured, err := tools.ReadMe(context.Background(), &mcp.CallToolRequest{}, ReadMeInput{Modules: []string{"chart"}})
	if err != nil {
		t.Fatal(err)
	}
	if structured != nil {
		t.Fatalf("structured = %#v, want nil; a non-nil structured output makes the SDK emit an outputSchema and clients drop the guide text", structured)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || text.Text != ReadMeGuide {
		t.Fatalf("content[0] must carry the full design-system guide, got %#v", result.Content[0])
	}
}

func TestReadMePlatformSchemaMatchesClaudeEnum(t *testing.T) {
	properties := readMeInputSchema()["properties"].(map[string]any)
	platform := properties["platform"].(map[string]any)
	enum, ok := platform["enum"].([]string)
	if !ok {
		t.Fatalf("platform schema missing enum: %#v", platform)
	}
	want := []string{"mobile", "desktop", "unknown"}
	if len(enum) != len(want) {
		t.Fatalf("platform enum = %#v, want %#v", enum, want)
	}
	for i := range want {
		if enum[i] != want[i] {
			t.Fatalf("platform enum = %#v, want %#v", enum, want)
		}
	}
	if _, ok := platform["default"]; ok {
		t.Fatalf("platform schema must describe, not encode, the default: %#v", platform)
	}
}
