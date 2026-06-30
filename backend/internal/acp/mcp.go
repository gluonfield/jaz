package acp

import (
	"context"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

const (
	MCPToolAgentSpawn  = "agent_spawn"
	MCPToolAgentSend   = "agent_send"
	MCPToolAgentStatus = "agent_status"
	MCPToolAgentWait   = "agent_wait"
	MCPToolAgentCancel = "agent_cancel"
	MCPToolAgentList   = "agent_list"
)

type MCPService interface {
	Spawn(context.Context, SpawnRequest) (SpawnResult, error)
	Send(context.Context, SendRequest) (Job, error)
	Status(string) (Job, error)
	Wait(context.Context, WaitRequest) (Job, error)
	Cancel(context.Context, string) (Job, error)
	List() []Job
	Agents() []string
}

type MCPTools struct {
	Service MCPService
}

func NewMCPTools(service MCPService) *MCPTools {
	return &MCPTools{Service: service}
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	agentNames := t.availableAgents()
	description := "Create an idle Jaz ACP agent session. Send work with agent_send."
	if len(agentNames) > 0 {
		description = "Create an idle Jaz ACP agent session. Use acp_agent or agent_name to choose one of: " + strings.Join(agentNames, ", ") + ". Empty uses the default selectable agent. Send work with agent_send."
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolAgentSpawn,
		Title:       "Spawn ACP agent",
		Description: description,
		InputSchema: spawnInputSchema(agentNames),
	}, t.Spawn)
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolAgentSend,
		Title:       "Send ACP agent task",
		Description: "Send a follow-up instruction to an idle Jaz ACP thread by thread id, thread slug, or active ACP session id. Selected @thread mentions expose a usable thread id.",
	}, t.Send)
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolAgentStatus,
		Title:       "Get ACP agent status",
		Description: "Get status and recent progress for a Jaz ACP thread by thread id, thread slug, or active ACP session id.",
	}, t.Status)
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolAgentWait,
		Title:       "Wait for ACP agent",
		Description: "Wait for a spawned Jaz ACP agent session to finish its current turn.",
	}, t.Wait)
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolAgentCancel,
		Title:       "Cancel ACP agent",
		Description: "Cancel a spawned Jaz ACP agent session's current turn.",
	}, t.Cancel)
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolAgentList,
		Title:       "List ACP agents",
		Description: "List active spawned Jaz ACP agent sessions and configured agent names.",
	}, t.List)
}

func (t *MCPTools) availableAgents() []string {
	return SelectableAgentNames(t.Service.Agents())
}

type MCPSpawnInput struct {
	ACPAgent        string `json:"acp_agent,omitempty"`
	AgentName       string `json:"agent_name,omitempty"`
	Slug            string `json:"slug,omitempty"`
	Title           string `json:"title,omitempty"`
	Directory       string `json:"directory,omitempty"`
	Worktree        bool   `json:"worktree,omitempty"`
	Branch          string `json:"branch,omitempty"`
	ModelProvider   string `json:"model_provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

func (t *MCPTools) Spawn(ctx context.Context, req *mcp.CallToolRequest, input MCPSpawnInput) (*mcp.CallToolResult, SpawnResult, error) {
	agent, err := ResolveAgentSelector(input.ACPAgent, input.AgentName)
	if err != nil {
		return nil, SpawnResult{}, err
	}
	result, err := t.Service.Spawn(ctx, SpawnRequest{
		ParentID:        mcpsession.SessionID(req),
		ACPAgent:        agent,
		Slug:            input.Slug,
		Title:           input.Title,
		Directory:       input.Directory,
		Worktree:        input.Worktree,
		Branch:          input.Branch,
		ModelProvider:   input.ModelProvider,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
	})
	return nil, result, err
}

type MCPSendInput struct {
	Session string `json:"session" jsonschema:"Jaz thread id, thread slug, or active ACP session id"`
	Message string `json:"message" jsonschema:"follow-up instruction to send"`
	Wait    bool   `json:"wait,omitempty" jsonschema:"wait for this turn to finish before returning; defaults to false"`
	Plan    bool   `json:"plan,omitempty" jsonschema:"request ACP plan mode for planning, review, or questions"`
}

func (t *MCPTools) Send(ctx context.Context, _ *mcp.CallToolRequest, input MCPSendInput) (*mcp.CallToolResult, Job, error) {
	completion := CompletionAsync
	if input.Wait {
		completion = CompletionInline
	}
	job, err := t.Service.Send(ctx, SendRequest{
		Session:       input.Session,
		Message:       input.Message,
		Completion:    completion,
		PlanRequested: input.Plan,
		ParentVisible: true,
	})
	if err != nil || !input.Wait {
		return nil, job, err
	}
	job, err = t.Service.Wait(ctx, WaitRequest{Session: job.ID})
	return nil, job, err
}

type MCPSessionInput struct {
	Session string `json:"session" jsonschema:"Jaz thread id, thread slug, or active ACP session id"`
}

func (t *MCPTools) Status(_ context.Context, _ *mcp.CallToolRequest, input MCPSessionInput) (*mcp.CallToolResult, Job, error) {
	job, err := t.Service.Status(input.Session)
	return nil, job, err
}

type MCPWaitInput struct {
	Session        string `json:"session" jsonschema:"active Jaz thread id, thread slug, or ACP session id"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"maximum seconds to wait; defaults to 30"`
}

func (t *MCPTools) Wait(ctx context.Context, _ *mcp.CallToolRequest, input MCPWaitInput) (*mcp.CallToolResult, Job, error) {
	job, err := t.Service.Wait(ctx, WaitRequest{
		Session: input.Session,
		Timeout: time.Duration(input.TimeoutSeconds) * time.Second,
	})
	return nil, job, err
}

func (t *MCPTools) Cancel(ctx context.Context, _ *mcp.CallToolRequest, input MCPSessionInput) (*mcp.CallToolResult, Job, error) {
	job, err := t.Service.Cancel(ctx, input.Session)
	return nil, job, err
}

type MCPListInput struct{}

type MCPListOutput struct {
	Agents   []string `json:"agents"`
	Sessions []Job    `json:"sessions"`
}

func (t *MCPTools) List(context.Context, *mcp.CallToolRequest, MCPListInput) (*mcp.CallToolResult, MCPListOutput, error) {
	return nil, MCPListOutput{
		Agents:   t.availableAgents(),
		Sessions: t.Service.List(),
	}, nil
}

func spawnInputSchema(agents []string) map[string]any {
	agentList := strings.Join(agents, ", ")
	agentDescription := "Configured Jaz ACP agent name."
	agentNameDescription := "Alias for acp_agent. Use this when the caller expects an agent_name field."
	if len(agents) > 0 {
		agentDescription += " Valid configured agents: " + agentList + ". Empty uses the default selectable agent."
		agentNameDescription += " Valid configured agents: " + agentList + "."
	} else {
		agentDescription += " No ACP agents are currently configured."
		agentNameDescription += " No ACP agents are currently configured."
	}
	agentProperty := map[string]any{
		"type":        "string",
		"description": agentDescription,
		"enum":        agents,
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"acp_agent": agentProperty,
			"agent_name": map[string]any{
				"type":        "string",
				"description": agentNameDescription,
				"enum":        agents,
			},
			"slug": map[string]any{
				"type":        "string",
				"description": "Stable human-readable handle for the spawned session.",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Optional display title for the spawned session.",
			},
			"directory": map[string]any{
				"type":        "string",
				"description": "Working directory for the agent, relative to the Jaz workspace.",
			},
			"worktree": map[string]any{
				"type":        "boolean",
				"description": "Run the session on a disposable git worktree of directory.",
			},
			"branch": map[string]any{
				"type":        "string",
				"description": "Base branch or ref for worktree=true.",
			},
			"model_provider": map[string]any{
				"type":        "string",
				"description": "Provider override for provider-backed agents.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Model override for this session. Omit unless the user asked for a specific model.",
			},
			"reasoning_effort": map[string]any{
				"type":        "string",
				"description": "Reasoning effort override: none, minimal, low, medium, high, or xhigh.",
			},
		},
	}
}
