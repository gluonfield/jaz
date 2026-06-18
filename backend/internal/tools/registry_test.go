package tools

import (
	"context"
	"testing"
)

type registryTestTool string

func (t registryTestTool) Definition() Definition {
	return Function(string(t), "test tool", false, ObjectSchema(nil, nil))
}

func (t registryTestTool) Execute(ctx context.Context, inputs map[string]any) (Result, error) {
	return Result{Content: "{}"}, nil
}

func TestRegistryInGroupTracksSetAndRemove(t *testing.T) {
	registry := NewRegistry(registryTestTool("direct"))
	registry.SetGroup("mcp", []Tool{registryTestTool("remote")})

	if !registry.InGroup("mcp", "remote") {
		t.Fatal("expected grouped tool to be in group")
	}
	if registry.InGroup("mcp", "direct") {
		t.Fatal("direct tool should not be in group")
	}

	registry.RemoveGroup("mcp")
	if registry.InGroup("mcp", "remote") {
		t.Fatal("removed grouped tool should not remain in group")
	}
}
