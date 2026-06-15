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
	for _, want := range []string{"Complete Reference", "SVG Setup", "visualize:show_widget"} {
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
	if !strings.Contains(result.Content, `"artifact_type":"svg"`) {
		t.Fatalf("result = %s", result.Content)
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
