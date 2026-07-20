package acp

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

type MCPService interface {
	Spawn(context.Context, SpawnRequest) (SpawnResult, error)
	Send(context.Context, SendRequest) (Job, error)
	Status(string) (Job, error)
	Wait(context.Context, WaitRequest) (Job, error)
	Cancel(context.Context, string) (Job, error)
	List() []Job
	Agents() []string
	AgentOptions(AgentOptionsRequest) (AgentOptionsOutput, error)
}

type MCPTools struct {
	Service MCPService
}

func NewMCPTools(service MCPService) *MCPTools {
	return &MCPTools{Service: service}
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	agentNames := t.availableAgents()
	jobSchema := mcpOutputSchema[Job]()
	description := "Create an idle Jaz agent session. Send work with jazagent_send. Omit model unless the user asks for a specific model; use jazagent_options({}) to inspect available agents and useful model choices."
	if len(agentNames) > 0 {
		description = "Create an idle Jaz agent session. Use acp_agent or agent_name to choose one of: " + strings.Join(agentNames, ", ") + ". Empty uses the default selectable agent. Send work with jazagent_send. Omit model unless the user asks for a specific model; use jazagent_options({}) to inspect available agents and useful model choices."
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolJazAgentSpawn,
		Title:       "Spawn Jaz agent",
		Description: description,
		InputSchema: spawnInputSchema(agentNames),
	}, t.Spawn)
	mcp.AddTool(server, &mcp.Tool{
		Name:         ToolJazAgentSend,
		Title:        "Send Jaz agent task",
		Description:  "Send an instruction to an idle Jaz agent session by thread id, thread slug, or active session id. Selected @thread mentions expose a usable thread id.",
		OutputSchema: jobSchema,
	}, t.Send)
	mcp.AddTool(server, &mcp.Tool{
		Name:         ToolJazAgentStatus,
		Title:        "Get Jaz agent status",
		Description:  "Get status and recent progress for a Jaz agent session by thread id, thread slug, or active session id.",
		OutputSchema: jobSchema,
	}, t.Status)
	mcp.AddTool(server, &mcp.Tool{
		Name:         ToolJazAgentWait,
		Title:        "Wait for Jaz agent",
		Description:  "Wait for a Jaz agent session to finish its current turn.",
		OutputSchema: jobSchema,
	}, t.Wait)
	mcp.AddTool(server, &mcp.Tool{
		Name:         ToolJazAgentCancel,
		Title:        "Cancel Jaz agent",
		Description:  "Cancel a Jaz agent session's current turn.",
		OutputSchema: jobSchema,
	}, t.Cancel)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolJazAgentOptions,
		Title:       "List Jaz agent options",
		Description: "List available Jaz agents and useful model choices. Call with empty input for the default shortlist. For huge provider catalogs such as OpenRouter, pass agent and name to search model names or ids.",
	}, t.Options)
	mcp.AddTool(server, &mcp.Tool{
		Name:         ToolJazAgentList,
		Title:        "List Jaz agents",
		Description:  "List active Jaz agent sessions.",
		OutputSchema: mcpOutputSchema[MCPListOutput](),
	}, t.List)
}

func mcpOutputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[json.RawMessage](): {},
	}})
	if err != nil {
		panic(err)
	}
	return schema
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
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"maximum seconds to wait; defaults to 600. On timeout returns the current snapshot with state still running"`
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

type MCPOptionsInput struct {
	Agent string `json:"agent,omitempty"`
	Name  string `json:"name,omitempty"`
}

func (t *MCPTools) Options(_ context.Context, _ *mcp.CallToolRequest, input MCPOptionsInput) (*mcp.CallToolResult, AgentOptionsOutput, error) {
	out, err := t.Service.AgentOptions(AgentOptionsRequest{Agent: input.Agent, Name: input.Name})
	return nil, out, err
}

type MCPListOutput struct {
	Sessions []Job `json:"sessions"`
}

type MCPListInput struct{}

func (t *MCPTools) List(_ context.Context, _ *mcp.CallToolRequest, _ MCPListInput) (*mcp.CallToolResult, MCPListOutput, error) {
	return nil, MCPListOutput{Sessions: t.Service.List()}, nil
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
				"description": "Stable human-readable handle for the external session.",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Optional display title for the external session.",
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
				"description": "Reasoning effort override. Omit to use the configured default; built-in agents default to xhigh when supported.",
				"enum":        []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "ultracode"},
			},
		},
	}
}
