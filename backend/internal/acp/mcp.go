package acp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

type MCPService interface {
	AskUser(context.Context, string, AskUserInput) (AskUserOutput, error)
	Start(context.Context, SpawnRequest, storage.QueuedMessage) (Job, error)
	Input(context.Context, SteerRequest) (Job, error)
	WaitThreads(context.Context, []string, time.Duration) (ThreadResults, error)
	Cancel(context.Context, string) (Job, error)
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "ask_user",
		Description: "Ask one or more questions in the current thread's UI and wait for the user's answers. Use for missing information, preferences, or decisions that change what you do next. Works outside plan mode. Provide short concrete options whenever useful. Ask only what is needed and do not request confirmation already given. Users may skip questions; returns only answered questions keyed by id, or cancelled when interrupted, including when the user writes in the chat instead: then act on what they wrote rather than asking again.",
	}, t.AskUser)
	mcp.AddTool(server, t.CreateDefinition(), t.Create)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolSendMessageToThread,
		Description: "Send a follow-up to a Jaz thread: start an idle thread or steer its running turn through the agent's native capability. A running turn keeps its mode and result routing. Unsupported steering returns an error without stopping the thread. The prompt becomes a visible user message. Use threadId from create_thread, list_threads, or search_threads. Returns after dispatch.",
	}, t.Send)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolWaitThreads,
		Description: "Get status or wait for up to eight Jaz threads. Set timeoutMs: 0 for an immediate snapshot; otherwise wait until any thread completes, fails, or needs user input. Default and maximum timeout is 50000 ms. Returns compact snapshots of all targets and per-thread errors. A timeout leaves work running.",
	}, t.Wait)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolStopThread,
		Description: "Stop the current turn in a Jaz thread. Its saved conversation remains available.",
	}, t.Cancel)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolListAgentOptions,
		Description: "List available Jaz agents and their model choices. Empty input returns the default shortlist. For large catalogs, pass agent and name to filter model names or IDs.",
	}, t.Options)
}

type MCPCreateInput struct {
	Prompt        string `json:"prompt" jsonschema:"Initial user prompt for the new thread."`
	Agent         string `json:"agent,omitempty" jsonschema:"Agent harness to use. Omit to use the configured default."`
	Slug          string `json:"slug,omitempty" jsonschema:"Optional human-readable thread handle."`
	Title         string `json:"title,omitempty" jsonschema:"Optional thread title."`
	Directory     string `json:"directory,omitempty" jsonschema:"Project directory on the server; relative paths resolve inside the Jaz workspace. Omit for a permanent directory named by thread ID."`
	Worktree      bool   `json:"worktree,omitempty" jsonschema:"Use a disposable Git worktree of directory."`
	Branch        string `json:"branch,omitempty" jsonschema:"Base ref for worktree. Omit to use the current HEAD."`
	ModelProvider string `json:"modelProvider,omitempty" jsonschema:"Optional provider override."`
	Model         string `json:"model,omitempty" jsonschema:"Model override. Omit unless the user requests a specific model."`
	Thinking      string `json:"thinking,omitempty" jsonschema:"Reasoning effort supported by the selected model. Omit to use its configured default."`
	Plan          bool   `json:"plan,omitempty" jsonschema:"Request the agent's plan mode for this prompt."`
	Commitment    int64  `json:"commitment,omitempty" jsonschema:"For a Jaz bot's own chat: the number of the open commitment this thread serves."`
}

func (t *MCPTools) AskUser(ctx context.Context, req *mcp.CallToolRequest, input AskUserInput) (*mcp.CallToolResult, AskUserOutput, error) {
	out, err := t.Service.AskUser(ctx, mcpsession.SessionID(req), input)
	return nil, out, err
}

func (t *MCPTools) CreateDefinition() *mcp.Tool {
	schema, err := jsonschema.For[MCPCreateInput](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["agent"].Enum = make([]any, 0)
	for _, agent := range t.availableAgents() {
		schema.Properties["agent"].Enum = append(schema.Properties["agent"].Enum, agent)
	}
	return &mcp.Tool{
		Name:        ToolCreateThread,
		Description: "Create a separate, saved Jaz conversation and start its initial prompt. A thread has an ID, message history, working directory, and agent settings, and appears in the user's sidebar. Use only when the user or your instructions ask for a separate thread or agent session. Returns after dispatch. Omit model unless the user requests one. Use list_agent_options for available agents and models.",
		InputSchema: schema,
	}
}

func (t *MCPTools) availableAgents() []string {
	return SelectableAgentNames(t.Service.Agents())
}

func (t *MCPTools) Create(ctx context.Context, req *mcp.CallToolRequest, input MCPCreateInput) (*mcp.CallToolResult, ThreadSnapshot, error) {
	if strings.TrimSpace(input.Prompt) == "" {
		return nil, ThreadSnapshot{}, fmt.Errorf("prompt is required")
	}
	parent := mcpsession.SessionID(req)
	job, err := t.Service.Start(ctx, SpawnRequest{
		ParentID:        parent,
		ACPAgent:        input.Agent,
		Slug:            input.Slug,
		Title:           input.Title,
		Directory:       input.Directory,
		Worktree:        input.Worktree,
		Branch:          input.Branch,
		ModelProvider:   input.ModelProvider,
		Model:           input.Model,
		ReasoningEffort: input.Thinking,
		Commitment:      input.Commitment,
	}, storage.QueuedMessage{Text: input.Prompt, PlanRequested: input.Plan, From: parent})
	if err != nil {
		return nil, ThreadSnapshot{}, err
	}
	return nil, snapshotThread(job), nil
}

type MCPSendInput struct {
	ThreadID string `json:"threadId" jsonschema:"Jaz thread ID or slug."`
	Prompt   string `json:"prompt" jsonschema:"Follow-up user prompt."`
	Plan     bool   `json:"plan,omitempty" jsonschema:"Request the agent's plan mode for this prompt."`
}

func (t *MCPTools) Send(ctx context.Context, req *mcp.CallToolRequest, input MCPSendInput) (*mcp.CallToolResult, ThreadSnapshot, error) {
	job, err := t.Service.Input(ctx, SteerRequest{
		Session: input.ThreadID, Message: input.Prompt, PlanRequested: input.Plan,
		Completion: CompletionAsync, ParentVisible: true, From: mcpsession.SessionID(req),
	})
	return nil, snapshotThread(job), err
}

type MCPThreadInput struct {
	ThreadID string `json:"threadId" jsonschema:"Jaz thread ID or slug."`
}

type MCPWaitInput struct {
	Targets   []MCPThreadInput `json:"targets" jsonschema:"One to eight threads to inspect or wait for."`
	TimeoutMs *int             `json:"timeoutMs,omitempty" jsonschema:"0 for immediate status; 1-50000 to wait. Defaults to 50000 milliseconds."`
}

// Claude Code aborts an MCP request that receives no response byte within 60 s.
const maxWaitMs = 50000

type MCPWaitOutput struct {
	Threads []ThreadSnapshot  `json:"threads"`
	Errors  map[string]string `json:"errors,omitempty"`
}

func (t *MCPTools) Wait(ctx context.Context, _ *mcp.CallToolRequest, input MCPWaitInput) (*mcp.CallToolResult, MCPWaitOutput, error) {
	timeout := maxWaitMs
	if input.TimeoutMs != nil {
		timeout = *input.TimeoutMs
	}
	if timeout < 0 || timeout > maxWaitMs {
		return nil, MCPWaitOutput{}, fmt.Errorf("timeoutMs must be between 0 and %d", maxWaitMs)
	}
	refs := make([]string, 0, len(input.Targets))
	for _, target := range input.Targets {
		refs = append(refs, target.ThreadID)
	}
	result, err := t.Service.WaitThreads(ctx, refs, time.Duration(timeout)*time.Millisecond)
	out := MCPWaitOutput{Threads: make([]ThreadSnapshot, 0, len(result.Threads)), Errors: result.Errors}
	for _, job := range result.Threads {
		out.Threads = append(out.Threads, snapshotThread(job))
	}
	return nil, out, err
}

func (t *MCPTools) Cancel(ctx context.Context, _ *mcp.CallToolRequest, input MCPThreadInput) (*mcp.CallToolResult, ThreadSnapshot, error) {
	job, err := t.Service.Cancel(ctx, input.ThreadID)
	return nil, snapshotThread(job), err
}

type MCPOptionsInput struct {
	Agent string `json:"agent,omitempty"`
	Name  string `json:"name,omitempty"`
}

func (t *MCPTools) Options(_ context.Context, _ *mcp.CallToolRequest, input MCPOptionsInput) (*mcp.CallToolResult, AgentOptionsOutput, error) {
	out, err := t.Service.AgentOptions(AgentOptionsRequest(input))
	return nil, out, err
}
