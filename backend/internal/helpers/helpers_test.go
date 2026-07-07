package helpers

import "testing"

func TestGenerateSchemaMatchesOpenAIStrictRequirements(t *testing.T) {
	type input struct {
		Required string `json:"required"`
		Optional string `json:"optional,omitempty"`
	}

	schema := GenerateSchema[input]()
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required = %#v", schema["required"])
	}
	if len(required) != 2 {
		t.Fatalf("required = %#v", required)
	}
	properties := schema["properties"].(map[string]any)
	optional := properties["optional"].(map[string]any)
	types := optional["type"].([]any)
	if len(types) != 2 || types[0] != "string" || types[1] != "null" {
		t.Fatalf("optional type = %#v", types)
	}
}

func TestGenerateSchemaAllowsNullForOptionalEnums(t *testing.T) {
	type input struct {
		Optional string `json:"optional,omitempty" jsonschema:"enum=low,enum=xhigh"`
	}

	schema := GenerateSchema[input]()
	properties := schema["properties"].(map[string]any)
	optional := properties["optional"].(map[string]any)
	values := optional["enum"].([]any)
	if len(values) != 3 || values[0] != "low" || values[1] != "xhigh" || values[2] != nil {
		t.Fatalf("optional enum = %#v", values)
	}
}
