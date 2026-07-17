package list

import (
	"context"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/tools"
)

type Tool struct {
	Manager *acp.Manager
}

type output struct {
	Sessions []acp.Job `json:"sessions"`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function(acp.ToolACPSessionList, "List active external ACP sessions.", true, map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []string{},
		"additionalProperties": false,
	})
}

func (t *Tool) Execute(_ context.Context, _ map[string]any) (tools.Result, error) {
	return tools.JSONResult(output{Sessions: t.Manager.List()})
}
