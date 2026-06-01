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
	Wait    bool   `json:"wait,omitempty" jsonschema_description:"Wait for this turn to finish before returning. Use for short commands; defaults to false."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function("agent_send", "Send a follow-up instruction to an idle spawned ACP agent session. Set wait=true for short requests where the result should come back synchronously.", true, helpers.GenerateSchema[input]())
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[input](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	completion := acp.CompletionAsync
	if req.Wait {
		completion = acp.CompletionInline
	}
	job, err := t.Manager.Send(ctx, acp.SendRequest{Session: req.Session, Message: req.Message, Completion: completion})
	if err != nil {
		return tools.Result{}, err
	}
	if req.Wait {
		job, err = t.Manager.Wait(ctx, acp.WaitRequest{Session: job.ID})
		if err != nil {
			return tools.Result{}, err
		}
	}
	return tools.JSONResult(job)
}
