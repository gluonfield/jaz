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
	ACPAgent  string `json:"acp_agent,omitempty" jsonschema_description:"Configured ACP agent name, for example codex or claude."`
	Slug      string `json:"slug,omitempty" jsonschema_description:"Stable human-readable handle for the spawned session."`
	Title     string `json:"title,omitempty" jsonschema_description:"Optional display title for the spawned session."`
	Directory string `json:"directory,omitempty" jsonschema_description:"Working directory for the agent, relative to the jaz workspace; created if missing. Set this when the user names an existing project, repo, or folder. Omit for ad-hoc tasks: a fresh directory named after the session is created."`
	Worktree  bool   `json:"worktree,omitempty" jsonschema_description:"Run the session on a disposable git worktree of directory (which must be a git repository), isolating its changes on a session branch."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function(
		"agent_spawn",
		"Create an idle ACP-backed agent session, such as codex or claude. This only creates the session; send tasks with agent_send and choose wait=true or wait=false per task. Pass directory to work inside an existing project; pass worktree=true to isolate repo changes on a session branch.",
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
		ParentID:  sessioncontext.SessionID(ctx),
		ACPAgent:  req.ACPAgent,
		Slug:      req.Slug,
		Title:     req.Title,
		Directory: req.Directory,
		Worktree:  req.Worktree,
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(result)
}
