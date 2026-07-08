package options

import (
	"testing"

	"github.com/wins/jaz/backend/internal/tools"
)

func TestDefinitionExposesAgentAndNameFilters(t *testing.T) {
	def := (&Tool{}).Definition()
	if tools.DefinitionName(def) != "agent_options" {
		t.Fatalf("tool name = %q", tools.DefinitionName(def))
	}
	params := map[string]any(def.GetFunction().Parameters)
	properties, _ := params["properties"].(map[string]any)
	for _, name := range []string{"agent", "name"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("schema missing %s: %#v", name, properties)
		}
	}
	required, _ := params["required"].([]string)
	if len(required) != 0 {
		t.Fatalf("agent_options should allow an empty call, required = %#v", params["required"])
	}
}
