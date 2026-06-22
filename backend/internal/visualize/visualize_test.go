package visualize

import (
	"context"
	"strings"
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

func TestReadMeFiltersToRequestedModules(t *testing.T) {
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
	if !ok {
		t.Fatalf("content[0] = %#v, want TextContent", result.Content[0])
	}
	guide := text.Text

	for _, want := range []string{"## Modules", "## Core Design System", "## Color palette", "## UI components", "## Charts (Chart.js)", "## Geographic maps (D3 choropleth)"} {
		if !strings.Contains(guide, want) {
			t.Fatalf("chart guide missing required section %q", want)
		}
	}
	for _, skip := range []string{"## SVG setup", "## Diagram types", "## Art and illustration", "## Elicitation"} {
		if strings.Contains(guide, skip) {
			t.Fatalf("chart guide should not include unrelated section %q", skip)
		}
	}
	// Stay well under the agent tool-result cap that spilled the full guide to a file.
	if len(guide) > 50_000 {
		t.Fatalf("chart guide = %d bytes, want under the tool-result budget", len(guide))
	}
}

func TestBuildReadMeGuideByModule(t *testing.T) {
	diagram := BuildReadMeGuide([]string{"diagram"}, "")
	if !strings.Contains(diagram, "## SVG setup") || !strings.Contains(diagram, "## Diagram types") {
		t.Fatal("diagram guide must include SVG setup and Diagram types")
	}
	if strings.Contains(diagram, "## Charts (Chart.js)") {
		t.Fatal("diagram guide must exclude chart sections")
	}

	core := BuildReadMeGuide(nil, "")
	if !strings.Contains(core, "## Modules") || !strings.Contains(core, "## Core Design System") {
		t.Fatal("empty-modules guide must keep the index and design-system core")
	}
	for _, skip := range []string{"## Charts (Chart.js)", "## Diagram types", "## Elicitation", "## UI components"} {
		if strings.Contains(core, skip) {
			t.Fatalf("empty-modules guide should exclude module section %q", skip)
		}
	}

	multi := BuildReadMeGuide([]string{"chart", "diagram"}, "")
	if !strings.Contains(multi, "## Charts (Chart.js)") || !strings.Contains(multi, "## Diagram types") {
		t.Fatal("multi-module guide must union the requested modules' sections")
	}
}

func TestReadMeGuideUsesMobileLayoutGuidance(t *testing.T) {
	desktop := BuildReadMeGuide([]string{"chart"}, "desktop")
	mobile := BuildReadMeGuide([]string{"chart"}, "mobile")
	if !strings.Contains(desktop, "The widget container is 680px wide.") {
		t.Fatalf("desktop guide missing 680px width:\n%s", desktop)
	}
	if strings.Contains(desktop, "Mobile column cap") {
		t.Fatalf("desktop guide must not include mobile column cap:\n%s", desktop)
	}
	for _, want := range []string{
		"The widget container is 380px wide.",
		"Mobile column cap",
		"never lay out more than TWO columns",
		"do not write `repeat(3, …)` or `repeat(4, …)`",
	} {
		if !strings.Contains(mobile, want) {
			t.Fatalf("mobile guide missing %q:\n%s", want, mobile)
		}
	}
}

func TestReadMePlatformSchemaIsPlainString(t *testing.T) {
	properties := readMeInputSchema()["properties"].(map[string]any)
	platform := properties["platform"].(map[string]any)
	if platform["type"] != "string" {
		t.Fatalf("platform schema = %#v, want string", platform)
	}
	if _, ok := platform["enum"]; ok {
		t.Fatalf("platform schema must stay a plain string, got enum: %#v", platform)
	}
	if _, ok := platform["default"]; ok {
		t.Fatalf("platform schema must not encode a default: %#v", platform)
	}
}
