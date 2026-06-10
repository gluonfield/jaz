package tools

import (
	"context"
	"encoding/json"

	oa "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/wins/jaz/backend/internal/media"
)

type Definition = oa.ChatCompletionToolUnionParam
type FunctionDefinition = shared.FunctionDefinitionParam
type FunctionParameters = shared.FunctionParameters

type Result struct {
	Content   string         `json:"content"`
	MediaRefs []media.Ref    `json:"media_refs,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Tool interface {
	Definition() Definition
	Execute(ctx context.Context, inputs map[string]any) (Result, error)
}

func Function(name, description string, strict bool, parameters map[string]any) Definition {
	return oa.ChatCompletionFunctionTool(FunctionDefinition{
		Name:        name,
		Description: oa.String(description),
		Strict:      oa.Bool(strict),
		Parameters:  FunctionParameters(parameters),
	})
}

func DefinitionName(def Definition) string {
	fn := def.GetFunction()
	if fn == nil {
		return ""
	}
	return fn.Name
}

func StringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func NumberSchema(description string) map[string]any {
	return map[string]any{
		"type":        "number",
		"description": description,
	}
}

func BoolSchema(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
	}
}

func ObjectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func StringInput(inputs map[string]any, key string) string {
	v, _ := inputs[key].(string)
	return v
}

func BoolInput(inputs map[string]any, key string, fallback bool) bool {
	v, ok := inputs[key].(bool)
	if !ok {
		return fallback
	}
	return v
}

func IntInput(inputs map[string]any, key string, fallback int) int {
	switch v := inputs[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return fallback
	}
}

func Clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func JSONResult(v any) (Result, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return Result{}, err
	}
	return Result{Content: string(b)}, nil
}
