package spawn

import (
	"context"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/helpers"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
)

type Tool struct {
	Manager *acp.Manager
}

type input struct {
	ACPAgent string `json:"acp_agent,omitempty" jsonschema_description:"Configured ACP agent name, for example codex or claude_code."`
	Slug     string `json:"slug,omitempty" jsonschema_description:"Stable human-readable handle for the spawned session."`
	Title    string `json:"title,omitempty" jsonschema_description:"Optional display title for the spawned session."`
	Message  string `json:"message" jsonschema_description:"Initial instruction to send to the ACP agent."`
	Cwd      string `json:"cwd,omitempty" jsonschema_description:"Working directory for the ACP session."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function(
		"agent_spawn",
		"Spawn an ACP-backed agent session and send it an initial instruction.",
		true,
		helpers.GenerateSchema[input](),
	)
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[input](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	result, err := t.Manager.Spawn(ctx, acp.SpawnRequest{
		ParentID: sessioncontext.SessionID(ctx),
		ACPAgent: req.ACPAgent,
		Slug:     req.Slug,
		Title:    req.Title,
		Message:  req.Message,
		Cwd:      req.Cwd,
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(result)
}
