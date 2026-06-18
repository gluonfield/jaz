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
	if got := tools.DefinitionName(readDef); got != visualizesvc.ReadMeMCPToolName {
		t.Fatalf("read tool name = %q, want %q", got, visualizesvc.ReadMeMCPToolName)
	}
	showDef := ShowWidgetTool{}.Definition()
	if got := tools.DefinitionName(showDef); got != visualizesvc.ShowWidgetMCPToolName {
		t.Fatalf("show tool name = %q, want %q", got, visualizesvc.ShowWidgetMCPToolName)
	}
}

func TestReadMeExecuteFiltersByModule(t *testing.T) {
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
		result, err := ReadMeTool{}.Execute(context.Background(), map[string]any{"modules": tc.modules})
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

func TestReadMeExecuteEmptyReturnsCore(t *testing.T) {
	result, err := ReadMeTool{}.Execute(context.Background(), nil)
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
