package jazagent

import (
	"testing"

	"github.com/wins/jaz/backend/internal/visualize"
	"github.com/wins/jaz/backend/internal/widgets"
)

func TestIncludeToolForArtifactSurface(t *testing.T) {
	tests := []struct {
		name    string
		surface string
		want    bool
	}{
		{visualize.ReadMeMCPToolName, "", true},
		{visualize.ShowWidgetMCPToolName, "", true},
		{widgets.PublishMCPToolName, "", false},
		{"mcp_jaztools_visualise_show_widget", "", true},
		{"mcp_jaztools_visualise_publish_widget", "", false},
		{visualize.ReadMeMCPToolName, string(visualize.SurfaceWidget), true},
		{visualize.ShowWidgetMCPToolName, string(visualize.SurfaceWidget), false},
		{widgets.PublishMCPToolName, string(visualize.SurfaceWidget), true},
		{"mcp_jaztools_visualise_show_widget", string(visualize.SurfaceWidget), false},
		{"mcp_jaztools_visualise_publish_widget", string(visualize.SurfaceWidget), true},
	}
	for _, tt := range tests {
		if got := includeToolForArtifactSurface(tt.name, tt.surface); got != tt.want {
			t.Fatalf("includeToolForArtifactSurface(%q, %q) = %v, want %v", tt.name, tt.surface, got, tt.want)
		}
	}
}
