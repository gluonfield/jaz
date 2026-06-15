package visualize

import (
	"context"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/tools"
	visualizesvc "github.com/wins/jaz/backend/internal/visualize"
)

func TestDefinitions(t *testing.T) {
	readDef := ReadMeTool{}.Definition()
	if got := tools.DefinitionName(readDef); got != visualizesvc.ReadMeToolName {
		t.Fatalf("read tool name = %q, want %q", got, visualizesvc.ReadMeToolName)
	}
	showDef := ShowWidgetTool{}.Definition()
	if got := tools.DefinitionName(showDef); got != visualizesvc.ShowWidgetToolName {
		t.Fatalf("show tool name = %q, want %q", got, visualizesvc.ShowWidgetToolName)
	}
}

func TestReadMeIncludesReferenceGuidance(t *testing.T) {
	result, err := ReadMeTool{}.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Imagine — Visual Creation Suite",
		"Call read_me again with the modules parameter",
		"Core Design System",
		"Color palette",
		"SVG setup",
		"The 680 in viewBox is load-bearing",
		"c-{ramp} nesting",
		"Diagram types",
		"UI components",
		"Charts (Chart.js)",
		"Geographic maps (D3 choropleth)",
		"Art and illustration",
		"Elicitation — collecting skill arguments",
		"Selected files appear as 120×120 tiles",
	} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("guide missing %q", want)
		}
	}
}

func TestShowWidgetAutoDetectsSVG(t *testing.T) {
	result, err := ShowWidgetTool{}.Execute(context.Background(), map[string]any{
		"loading_messages": []any{"Rendering"},
		"title":            "Flow",
		"widget_code":      `<svg width="100%" viewBox="0 0 680 120" role="img"></svg>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, visualizesvc.RenderedMessage) {
		t.Fatalf("result = %s", result.Content)
	}
	if result.Metadata["artifact_type"] != "svg" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if _, ok := result.Metadata["bytes"]; ok {
		t.Fatalf("metadata must not expose storage/debug fields: %#v", result.Metadata)
	}
}

func TestShowWidgetRequiresLoadingMessages(t *testing.T) {
	_, err := ShowWidgetTool{}.Execute(context.Background(), map[string]any{
		"title":       "Flow",
		"widget_code": "<div></div>",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
