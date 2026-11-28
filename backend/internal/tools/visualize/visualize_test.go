package visualize

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	visualizesvc "github.com/wins/jaz/backend/internal/visualize"
)

func TestDefinitions(t *testing.T) {
	readDef := GuideTool{}.Definition()
	if got := tools.DefinitionName(readDef); got != visualizesvc.GuideMCPToolName {
		t.Fatalf("read tool name = %q, want %q", got, visualizesvc.GuideMCPToolName)
	}
	showDef := ShowArtifactTool{}.Definition()
	if got := tools.DefinitionName(showDef); got != visualizesvc.ShowArtifactMCPToolName {
		t.Fatalf("show tool name = %q, want %q", got, visualizesvc.ShowArtifactMCPToolName)
	}
}

func TestGuideExecuteFiltersByModule(t *testing.T) {
	cases := []struct {
		modules []any
		want    []string
		skip    []string
	}{
		{
			modules: []any{"diagram"},
			want:    []string{"# Imagine — Visual Creation Suite", "Core Design System", "Color palette", "SVG setup", "The 680 in viewBox is load-bearing", "c-{ramp} nesting", "Diagram types"},
			skip:    []string{"Charts (Chart.js)", "Elicitation — collecting skill arguments"},
		},
		{
			modules: []any{"chart"},
			want:    []string{"Core Design System", "UI components", "Charts (Chart.js)", "Geographic maps (D3 choropleth)"},
			skip:    []string{"Diagram types", "Elicitation — collecting skill arguments"},
		},
		{
			modules: []any{"elicitation"},
			want:    []string{"Elicitation — collecting skill arguments", "Selected files appear as 120×120 tiles"},
			skip:    []string{"Diagram types", "Charts (Chart.js)"},
		},
	}
	for _, tc := range cases {
		result, err := GuideTool{Visualize: &visualizesvc.Service{}}.Execute(context.Background(), map[string]any{"modules": tc.modules})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range tc.want {
			if !strings.Contains(result.Content, want) {
				t.Fatalf("modules %v: guide missing %q", tc.modules, want)
			}
		}
		for _, skip := range tc.skip {
			if strings.Contains(result.Content, skip) {
				t.Fatalf("modules %v: guide should exclude %q", tc.modules, skip)
			}
		}
	}
}

func TestGuideExecuteEmptyReturnsCore(t *testing.T) {
	result, err := GuideTool{Visualize: &visualizesvc.Service{}}.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "Core Design System") {
		t.Fatal("empty modules must still return the design-system core")
	}
	for _, skip := range []string{"Diagram types", "Charts (Chart.js)", "Elicitation — collecting skill arguments"} {
		if strings.Contains(result.Content, skip) {
			t.Fatalf("empty modules should exclude module section %q", skip)
		}
	}
}

func TestNativeCanvasDiscoveryAndPresentation(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	boards := canvas.NewService(runtimefiles.New(store.RootDir()))
	bus := sessionevents.New()
	events := bus.Subscribe(t.Context(), session.ID)
	service := visualizesvc.NewService(store, store, bus, boards)
	read := GuideTool{Visualize: service}
	show := ShowCanvasTool{Visualize: service}
	input := map[string]any{"modules": []any{"canvas", "diagram"}}
	if _, err := read.Execute(t.Context(), input); err == nil {
		t.Fatal("canvas guidance accepted a turn without a session")
	}
	ctx := sessioncontext.WithSessionID(t.Context(), session.ID)
	result, err := read.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(boards.Dir(session.ID), canvas.FileName)
	for _, want := range []string{file, "show_canvas", "Diagram types"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("native guide missing %q", want)
		}
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("discovery created a canvas: %v", err)
	}
	if _, err := show.Execute(ctx, nil); err == nil {
		t.Fatal("presentation accepted a missing canvas")
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"pages":[{"id":"plan","name":"Plan","nodes":[],"edges":[]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = show.Execute(ctx, nil)
	if err != nil || result.Content != visualizesvc.SavedMessage+"\n\n"+visualizesvc.DisplayReminder {
		t.Fatalf("native presentation = %+v, %v", result, err)
	}
	select {
	case event := <-events:
		if event.Artifact == nil || event.Artifact.Kind != sessionevents.PresentationCanvas || !event.Artifact.Presented || !event.Delivers() {
			t.Fatalf("native event = %+v", event)
		}
		stored, err := store.LoadSessionEvents(session.ID)
		if err != nil || len(stored) != 1 || stored[0].Seq != event.Seq {
			t.Fatalf("native presentation not persisted: %+v, %v", stored, err)
		}
	default:
		t.Fatal("native presentation did not publish")
	}
}

func TestShowArtifactAutoDetectsSVG(t *testing.T) {
	result, err := ShowArtifactTool{}.Execute(context.Background(), map[string]any{
		"loading_messages": []any{"Rendering"},
		"title":            "Flow",
		"html":             `<svg width="100%" viewBox="0 0 680 120" role="img"></svg>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "Artifact prepared for display in the transcript.") || !strings.Contains(result.Content, "Client rendering has not been confirmed.") {
		t.Fatalf("result = %s", result.Content)
	}
	if result.Metadata["artifact_type"] != "svg" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if _, ok := result.Metadata["bytes"]; ok {
		t.Fatalf("metadata must not expose storage/debug fields: %#v", result.Metadata)
	}
}

func TestShowArtifactRequiresLoadingMessages(t *testing.T) {
	_, err := ShowArtifactTool{}.Execute(context.Background(), map[string]any{
		"title": "Flow",
		"html":  "<div></div>",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
