package send

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
	Session string `json:"session" jsonschema_description:"Spawned session id or slug."`
	Message string `json:"message" jsonschema_description:"Follow-up instruction to send."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function("agent_send", "Send a follow-up instruction to an idle spawned ACP agent session.", true, helpers.GenerateSchema[input]())
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[input](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	job, err := t.Manager.Send(ctx, acp.SendRequest{Session: req.Session, Message: req.Message})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(job)
}
