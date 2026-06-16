package widget

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/tools"
)

func TestDefinitionPointsAtArtifactGuidance(t *testing.T) {
	def := (&Tool{}).Definition()
	if got := tools.DefinitionName(def); got != ToolName {
		t.Fatalf("tool name = %q", got)
	}
	fn := def.GetFunction()
	if fn == nil {
		t.Fatal("missing function definition")
	}
	desc := fn.Description.Value
	for _, want := range []string{"jaztools MCP artifact guidance", "visualize:read_me"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description missing %q: %s", want, desc)
		}
	}
}
