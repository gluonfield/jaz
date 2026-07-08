package list

import (
	"testing"

	"github.com/wins/jaz/backend/internal/tools"
)

func TestDefinitionHasNoFilters(t *testing.T) {
	def := (&Tool{}).Definition()
	if tools.DefinitionName(def) != "agent_list" {
		t.Fatalf("tool name = %q", tools.DefinitionName(def))
	}
	params := map[string]any(def.GetFunction().Parameters)
	properties, _ := params["properties"].(map[string]any)
	if len(properties) != 0 {
		t.Fatalf("agent_list properties = %#v", properties)
	}
}
