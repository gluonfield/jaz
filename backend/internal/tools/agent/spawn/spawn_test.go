package spawn

import (
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
	if _, ok := effort["enum"]; ok {
		t.Fatalf("reasoning_effort must be model-scoped, got global enum %#v", effort["enum"])
	}
	if tools.DefinitionName(def) != "acp_session_create" {
		t.Fatalf("tool name = %q", tools.DefinitionName(def))
	}
}
