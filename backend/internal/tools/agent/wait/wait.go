package wait

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/helpers"
	"github.com/wins/jaz/backend/internal/tools"
)

type Tool struct {
	Manager *acp.Manager
}

type input struct {
	Session        string `json:"session" jsonschema_description:"Spawned session id or slug."`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema_description:"Maximum seconds to wait. Defaults to 30."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function("agent_wait", "Wait for a spawned ACP agent session to finish its current turn.", true, helpers.GenerateSchema[input]())
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[input](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	job, err := t.Manager.Wait(ctx, acp.WaitRequest{
		Session: req.Session,
		Timeout: time.Duration(req.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(job)
}
