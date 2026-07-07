package spawn

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/internal/tools"
)

func TestDefinitionExposesAgentAndModelSelectors(t *testing.T) {
	def := (&Tool{}).Definition()
	fn := def.GetFunction()
	if fn == nil {
		t.Fatal("missing function definition")
	}
	params := map[string]any(fn.Parameters)
	properties, _ := params["properties"].(map[string]any)
	for _, name := range []string{"acp_agent", "agent_name", "model_provider", "model", "reasoning_effort"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("schema missing %s: %#v", name, properties)
		}
	}
	effort, _ := properties["reasoning_effort"].(map[string]any)
	effortEnum, _ := effort["enum"].([]any)
	if len(effortEnum) == 0 || !slices.Contains(effortEnum, any("xhigh")) || !slices.Contains(effortEnum, any("ultracode")) {
		t.Fatalf("reasoning_effort enum = %#v", effort["enum"])
	}
	if tools.DefinitionName(def) != "agent_spawn" {
		t.Fatalf("tool name = %q", tools.DefinitionName(def))
	}
}
