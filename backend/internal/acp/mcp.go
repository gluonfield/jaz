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
		Description: "Send a visible follow-up: start an idle thread or steer its running turn. Unsupported steering returns an error. Returns after dispatch.",
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
	Prompt        string `json:"prompt"`
	Agent         string `json:"agent,omitempty"`
	Slug          string `json:"slug,omitempty"`
	Title         string `json:"title,omitempty"`
	Directory     string `json:"directory,omitempty" jsonschema:"Server path; relative to workspace."`
	Worktree      bool   `json:"worktree,omitempty"`
	Branch        string `json:"branch,omitempty"`
	ModelProvider string `json:"modelProvider,omitempty"`
	Model         string `json:"model,omitempty"`
	Thinking      string `json:"thinking,omitempty"`
	Plan          bool   `json:"plan,omitempty"`
	Commitment    int64  `json:"task,omitempty" jsonschema:"Bot task id."`
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
	return &mcp.Tool{
		Name:        ToolCreateThread,
		Description: "Start an authorized saved thread; reuse work. Omit model unless requested. Returns after dispatch.",
		InputSchema: schema,
	}
}

func (t *MCPTools) Create(ctx context.Context, req *mcp.CallToolRequest, input MCPCreateInput) (*mcp.CallToolResult, ThreadSnapshot, error) {
	if strings.TrimSpace(input.Prompt) == "" {
		return nil, ThreadSnapshot{}, fmt.Errorf("prompt is required")
	}
	parent := mcpsession.SessionID(req)
	spawn := SpawnRequest{
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
	}
	first := storage.QueuedMessage{Text: input.Prompt, PlanRequested: input.Plan, From: parent}
	var job Job
	var err error
	var budget *BudgetDecision
	if service, ok := t.Service.(interface {
		StartBudgeted(context.Context, SpawnRequest, storage.QueuedMessage) (Job, BudgetDecision, error)
	}); ok {
		decision := BudgetDecision{}
		job, decision, err = service.StartBudgeted(ctx, spawn, first)
		budget = &decision
	} else {
		job, err = t.Service.Start(ctx, spawn, first)
	}
	if err != nil {
		return nil, ThreadSnapshot{}, err
	}
	snapshot := snapshotThread(job)
	snapshot.Budget = budget
	return nil, snapshot, nil
}

type MCPSendInput struct {
	ThreadID   string `json:"threadId" jsonschema:"Thread id or slug."`
	Prompt     string `json:"prompt"`
	Plan       bool   `json:"plan,omitempty"`
	Commitment int64  `json:"task,omitempty" jsonschema:"Bot task id."`
}

func (t *MCPTools) Send(ctx context.Context, req *mcp.CallToolRequest, input MCPSendInput) (*mcp.CallToolResult, ThreadSnapshot, error) {
	job, err := t.Service.Input(ctx, SteerRequest{
		Session: input.ThreadID, Message: input.Prompt, PlanRequested: input.Plan,
		ParentVisible: true, From: mcpsession.SessionID(req), Commitment: input.Commitment,
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
