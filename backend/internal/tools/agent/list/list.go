package list

import (
	"context"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/helpers"
	"github.com/wins/jaz/backend/internal/tools"
)

type Tool struct {
	Manager *acp.Manager
}

type input struct{}

func (t *Tool) Definition() tools.Definition {
	return tools.Function("agent_list", "List active spawned ACP agent sessions globally.", true, helpers.GenerateSchema[input]())
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	return tools.JSONResult(map[string]any{"sessions": t.Manager.List()})
}
