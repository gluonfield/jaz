package helpers

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

func GenerateSchema[T any]() map[string]any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect(new(T))
	data, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{"type": "object", "additionalProperties": false}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{"type": "object", "additionalProperties": false}
	}
	normalizeStrictSchema(out)
	return out
}

func DecodeMap[T any](inputs map[string]any) (T, error) {
	var out T
	data, err := json.Marshal(inputs)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}

func normalizeStrictSchema(schema map[string]any) {
	delete(schema, "$schema")
	delete(schema, "$id")
	if schema["type"] == "object" {
		schema["additionalProperties"] = false
		properties, _ := schema["properties"].(map[string]any)
		requiredSet := map[string]bool{}
		if required, ok := schema["required"].([]any); ok {
			for _, key := range required {
				if s, ok := key.(string); ok {
					requiredSet[s] = true
				}
			}
		}
		required := make([]string, 0, len(properties))
		for name, raw := range properties {
			required = append(required, name)
			prop, _ := raw.(map[string]any)
			if prop == nil {
				continue
			}
			normalizeStrictSchema(prop)
			if !requiredSet[name] {
				allowNull(prop)
			}
		}
		schema["required"] = required
	}
	if items, ok := schema["items"].(map[string]any); ok {
		normalizeStrictSchema(items)
	}
}

func allowNull(schema map[string]any) {
	switch typ := schema["type"].(type) {
	case string:
		if typ != "null" {
			schema["type"] = []any{typ, "null"}
		}
	case []any:
		for _, item := range typ {
			if item == "null" {
				return
			}
		}
		schema["type"] = append(typ, "null")
	}
}
