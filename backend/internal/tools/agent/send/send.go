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
	Plan    bool   `json:"plan,omitempty" jsonschema_description:"Set true when asking the ACP agent to make, propose, review, or revise a plan. Leave false when sending an approved plan for execution. This is the only way to request ACP plan mode from a Jaz session."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function("agent_send", "Send a follow-up instruction to an idle spawned ACP agent session. Set plan=true when the child should use ACP plan mode instead of answering with ordinary text; set wait=true for short requests where the result should come back synchronously.", true, helpers.GenerateSchema[input]())
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
	job, err := t.Manager.Send(ctx, acp.SendRequest{
		Session:       req.Session,
		Message:       req.Message,
		Completion:    completion,
		PlanRequested: req.Plan,
		ParentVisible: true,
	})
	if err != nil {
		return tools.Result{}, err
	}
	if req.Wait {
		job, err = t.Manager.Wait(ctx, acp.WaitRequest{Session: job.ID})
		if err != nil {
			return tools.Result{}, err
		}
	}
	if req.Plan {
		return tools.JSONResult(planModeResult(job))
	}
	return tools.JSONResult(job)
}

func planModeResult(job acp.Job) map[string]any {
	status := "sent"
	if job.State == acp.StateIdle || job.State == acp.StateCancelled || job.State == acp.StateFailed {
		status = "ready"
	}
	return map[string]any{
		"status":          status,
		"session_id":      job.ID,
		"slug":            job.Slug,
		"title":           job.Title,
		"acp_agent":       job.ACPAgent,
		"state":           job.State,
		"mode":            job.Modes.CurrentModeID,
		"has_plan":        len(job.Plan) > 0,
		"has_questions":   len(job.Permissions) > 0,
		"tool_call_count": len(job.ToolCalls),
		"rendered_by_jaz": true,
	}
}
