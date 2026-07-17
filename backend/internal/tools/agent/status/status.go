package status

import (
	"context"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/helpers"
	"github.com/wins/jaz/backend/internal/tools"
)

type Tool struct {
	Manager *acp.Manager
}

type input struct {
	Session string `json:"session" jsonschema_description:"External ACP session id or slug."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function(acp.ToolACPSessionStatus, "Get status and recent progress for an external ACP session.", true, helpers.GenerateSchema[input]())
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[input](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	job, err := t.Manager.Status(req.Session)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(job)
}
