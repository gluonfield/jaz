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
	ACPAgent        string `json:"acp_agent,omitempty" jsonschema_description:"Configured ACP agent name, for example codex, claude, grok, opencode, or antigravity. Empty uses the default selectable agent."`
	AgentName       string `json:"agent_name,omitempty" jsonschema_description:"Alias for acp_agent. Use this when the caller expects an agent_name field."`
	Slug            string `json:"slug,omitempty" jsonschema_description:"Stable human-readable handle for the spawned session."`
	Title           string `json:"title,omitempty" jsonschema_description:"Optional display title for the spawned session."`
	Directory       string `json:"directory,omitempty" jsonschema_description:"Working directory for the agent, relative to the jaz workspace; created if missing. Set this when the user names an existing project, repo, or folder. Omit for ad-hoc tasks: a fresh directory named after the session is created."`
	Worktree        bool   `json:"worktree,omitempty" jsonschema_description:"Run the session on a disposable git worktree of directory (which must be a git repository), isolating its changes on a session branch."`
	Branch          string `json:"branch,omitempty" jsonschema_description:"Base branch or ref for the disposable worktree. Only valid with worktree=true. Omit to branch from directory's current HEAD."`
	ModelProvider   string `json:"model_provider,omitempty" jsonschema_description:"Provider override for provider-backed agents."`
	Model           string `json:"model,omitempty" jsonschema_description:"Model override for this session. Omit unless the user asked for a specific model."`
	ReasoningEffort string `json:"reasoning_effort,omitempty" jsonschema:"enum=none,enum=minimal,enum=low,enum=medium,enum=high,enum=xhigh,enum=max,enum=ultracode" jsonschema_description:"Reasoning effort override. Omit to use the configured default; built-in agents default to xhigh when supported."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function(
		"agent_spawn",
		"Create an idle ACP-backed agent session, such as codex, claude, grok, opencode, or antigravity. Use acp_agent or agent_name to choose the agent; empty uses the default selectable agent. This only creates the session; send tasks with agent_send and choose wait=true or wait=false per task. Omit model unless the user asks for a specific model; use agent_options to inspect configured agents and model/provider options. Invalid models fail without creating a child thread. Pass directory to work inside an existing project; pass worktree=true to isolate repo changes on a session branch. With worktree=true, branch optionally chooses the base branch/ref; omit it to branch from directory's current HEAD.",
		true,
		helpers.GenerateSchema[input](),
	)
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[input](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	agent, err := acp.ResolveAgentSelector(req.ACPAgent, req.AgentName)
	if err != nil {
		return tools.Result{}, err
	}
	result, err := t.Manager.Spawn(ctx, acp.SpawnRequest{
		ParentID:        sessioncontext.SessionID(ctx),
		ACPAgent:        agent,
		Slug:            req.Slug,
		Title:           req.Title,
		Directory:       req.Directory,
		Worktree:        req.Worktree,
		Branch:          req.Branch,
		ModelProvider:   req.ModelProvider,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(result)
}
