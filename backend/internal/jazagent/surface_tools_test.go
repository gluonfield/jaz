package jazagent

import (
	"testing"

	"github.com/wins/jaz/backend/internal/boards"
	"github.com/wins/jaz/backend/internal/visualize"
)

func TestIncludeToolForArtifactSurface(t *testing.T) {
	tests := []struct {
		name    string
		surface string
		want    bool
	}{
		{visualize.GuideMCPToolName, "", true},
		{visualize.ShowArtifactMCPToolName, "", true},
		{boards.PublishMCPToolName, "", false},
		{"mcp_jaztools_show_artifact", "", true},
		{"mcp_jaztools_publish_app", "", true},
		{visualize.GuideMCPToolName, string(visualize.SurfaceWidget), true},
		{visualize.ShowArtifactMCPToolName, string(visualize.SurfaceWidget), false},
		{boards.PublishMCPToolName, string(visualize.SurfaceWidget), true},
		{"mcp_jaztools_show_artifact", string(visualize.SurfaceWidget), false},
		{"mcp_jaztools_publish_app", string(visualize.SurfaceWidget), false},
	}
	for _, tt := range tests {
		if got := includeToolForArtifactSurface(tt.name, tt.surface); got != tt.want {
			t.Fatalf("includeToolForArtifactSurface(%q, %q) = %v, want %v", tt.name, tt.surface, got, tt.want)
		}
	}
}
