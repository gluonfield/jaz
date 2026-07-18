package options

import (
	"testing"

	"github.com/wins/jaz/backend/internal/tools"
)

func TestDefinitionExposesAgentAndNameFilters(t *testing.T) {
	def := (&Tool{}).Definition()
	if tools.DefinitionName(def) != "jazagent_options" {
		t.Fatalf("tool name = %q", tools.DefinitionName(def))
	}
	params := map[string]any(def.GetFunction().Parameters)
	properties, _ := params["properties"].(map[string]any)
	for _, name := range []string{"agent", "name"} {
		property, ok := properties[name].(map[string]any)
		if !ok {
			t.Fatalf("schema missing %s: %#v", name, properties)
		}
		if description, _ := property["description"].(string); description == "" {
			t.Fatalf("schema property %s has no description", name)
		}
	}
	if required, ok := params["required"].([]any); ok && len(required) != 0 {
		t.Fatalf("jazagent_options should allow an empty call, required = %#v", params["required"])
	}
}
