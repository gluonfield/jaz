package visualize_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/visualize"
)

func TestShowWidgetSavesAndPublishesTheSameDelivery(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(session.ID, storage.Turn{Output: &storage.TurnOutput{User: true}}); err != nil {
		t.Fatal(err)
	}
	bus := sessionevents.New()
	events := bus.Subscribe(t.Context(), session.ID)
	tools := visualize.NewMCPTools(store, store, bus)
	result, structured, err := tools.ShowWidget(t.Context(), &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: mcpsession.Header(session.ID)},
	}, widgetInput())
	if err != nil {
		t.Fatal(err)
	}
	if structured != nil || len(result.Content) != 2 || result.Content[0].(*mcp.TextContent).Text != "Artifact saved in this conversation for display. Client rendering has not been confirmed." || !strings.Contains(result.Content[1].(*mcp.TextContent).Text, "Do not claim the user has already seen it.") {
		t.Fatalf("delivery receipt = %#v, %#v", result, structured)
	}
	page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14, ChatOnly: true})
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("chat history = %+v, %v", page, err)
	}
	saved := page.Events[0]
	if saved.Seq == 0 || saved.SessionID != session.ID || saved.Artifact == nil || saved.Artifact.Title != "System map" || saved.Artifact.WidgetCode != widgetInput().WidgetCode || !saved.Artifact.Presented {
		t.Fatalf("saved artifact = %+v", saved)
	}
	select {
	case live := <-events:
		if live.Seq != saved.Seq || live.SessionID != saved.SessionID || live.At.UnixMilli() != saved.At.UnixMilli() || !reflect.DeepEqual(live.Artifact, saved.Artifact) {
			t.Fatalf("live and history differ: %+v, %+v", live, saved)
		}
	default:
		t.Fatal("saved artifact was not published")
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil || !loaded.Turn.Output.Shown {
		t.Fatalf("artifact left the turn undelivered: %+v, %v", loaded.Turn, err)
	}
	tools.Store = rejectedEvents{}
	result, _, err = tools.ShowWidget(t.Context(), &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(session.ID)}}, widgetInput())
	if err == nil || result != nil {
		t.Fatalf("failed save got a success receipt: %+v, %v", result, err)
	}
	select {
	case event := <-events:
		t.Fatalf("failed save published an artifact: %+v", event)
	default:
	}
}

type rejectedEvents struct{}

func (rejectedEvents) AppendSessionEvents(string, ...sessionevents.Event) error {
	return errors.New("write failed")
}

func TestShowWidgetDoesNotClaimDeliveryWithoutASessionOrForPrivateTurns(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tools := visualize.NewMCPTools(store, store, sessionevents.New())
	for _, id := range []string{"", "missing"} {
		result, _, err := tools.ShowWidget(t.Context(), &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(id)}}, widgetInput())
		if err == nil || result != nil {
			t.Fatalf("session %q got a success receipt: %+v, %v", id, result, err)
		}
	}
	for _, source := range []string{storage.SourceBot, storage.SourceBotMember} {
		session, err := store.CreateSession(storage.CreateSession{SourceType: source})
		if err != nil {
			t.Fatal(err)
		}
		turn := storage.Turn{Output: &storage.TurnOutput{ReplyTo: "another-bot"}}
		if source == storage.SourceBotMember {
			turn.Output.ReplyTo = ""
		}
		if err := store.StartSessionTurn(session.ID, turn); err != nil {
			t.Fatal(err)
		}
		result, _, err := tools.ShowWidget(t.Context(), &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(session.ID)}}, widgetInput())
		if err != nil || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "not displayed") {
			t.Fatalf("private receipt = %+v, %v", result, err)
		}
		page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14, ChatOnly: true})
		if err != nil || len(page.Events) != 0 {
			t.Fatalf("private artifact leaked into chat: %+v, %v", page, err)
		}
		loaded, err := store.LoadSession(session.ID)
		if err != nil || loaded.Turn.Output.Shown {
			t.Fatalf("private artifact counted as delivery: %+v, %v", loaded.Turn, err)
		}
	}
}

func widgetInput() visualize.ShowWidgetInput {
	return visualize.ShowWidgetInput{
		LoadingMessages: []string{"Rendering"},
		Title:           "System map",
		WidgetCode:      `<svg width="100%" viewBox="0 0 680 120" role="img"><title>Map</title><desc>System map</desc></svg>`,
	}
}

func TestReadMeFiltersToRequestedModules(t *testing.T) {
	tools := visualize.NewMCPTools(nil, nil, nil)
	result, structured, err := tools.ReadMe(context.Background(), &mcp.CallToolRequest{}, visualize.ReadMeInput{Modules: []string{"chart"}})
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
	diagram := visualize.BuildReadMeGuide([]string{"diagram"}, "")
	if !strings.Contains(diagram, "## SVG setup") || !strings.Contains(diagram, "## Diagram types") {
		t.Fatal("diagram guide must include SVG setup and Diagram types")
	}
	if strings.Contains(diagram, "## Charts (Chart.js)") {
		t.Fatal("diagram guide must exclude chart sections")
	}

	core := visualize.BuildReadMeGuide(nil, "")
	if !strings.Contains(core, "## Modules") || !strings.Contains(core, "## Core Design System") {
		t.Fatal("empty-modules guide must keep the index and design-system core")
	}
	for _, skip := range []string{"## Charts (Chart.js)", "## Diagram types", "## Elicitation", "## UI components"} {
		if strings.Contains(core, skip) {
			t.Fatalf("empty-modules guide should exclude module section %q", skip)
		}
	}

	multi := visualize.BuildReadMeGuide([]string{"chart", "diagram"}, "")
	if !strings.Contains(multi, "## Charts (Chart.js)") || !strings.Contains(multi, "## Diagram types") {
		t.Fatal("multi-module guide must union the requested modules' sections")
	}
}

func TestReadMeGuideUsesMobileLayoutGuidance(t *testing.T) {
	desktop := visualize.BuildReadMeGuide([]string{"chart"}, "desktop")
	mobile := visualize.BuildReadMeGuide([]string{"chart"}, "mobile")
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
	properties := visualize.ReadMeInputSchema()["properties"].(map[string]any)
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
