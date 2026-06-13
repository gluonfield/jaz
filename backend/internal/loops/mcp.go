package loops

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPService interface {
	Create(CreateLoop) (Loop, error)
	Update(string, UpdateLoop) (Loop, error)
	Delete(string) error
	Load(string) (Loop, error)
	List() ([]Loop, error)
	Runs(string, int) ([]Run, error)
	RunNow(context.Context, string) (Run, error)
}

type MCPTools struct {
	service MCPService
}

func NewMCPTools(service MCPService) *MCPTools {
	return &MCPTools{service: service}
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_list",
		Title:       "List Jaz loops",
		Description: "List active Jaz loops.",
	}, t.List)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_get",
		Title:       "Get Jaz loop",
		Description: "Get one Jaz loop and its recent runs.",
	}, t.Get)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_create",
		Title:       "Create Jaz loop",
		Description: "Create a scheduled Jaz loop.",
	}, t.Create)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_update",
		Title:       "Update Jaz loop",
		Description: "Update an existing Jaz loop.",
	}, t.Update)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_run",
		Title:       "Run Jaz loop now",
		Description: "Start a manual run for a Jaz loop.",
	}, t.Run)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_delete",
		Title:       "Delete Jaz loop",
		Description: "Soft-delete a Jaz loop.",
	}, t.Delete)
}

type MCPListInput struct{}

type MCPListOutput struct {
	Loops []Loop `json:"loops"`
}

func (t *MCPTools) List(context.Context, *mcp.CallToolRequest, MCPListInput) (*mcp.CallToolResult, MCPListOutput, error) {
	items, err := t.service.List()
	return nil, MCPListOutput{Loops: items}, err
}

type MCPIDInput struct {
	ID    string `json:"id" jsonschema:"loop id"`
	Limit int    `json:"limit,omitempty" jsonschema:"recent run limit, 1-100, default 20"`
}

type MCPDetailOutput struct {
	Loop Loop  `json:"loop"`
	Runs []Run `json:"runs,omitempty"`
}

func (t *MCPTools) Get(_ context.Context, _ *mcp.CallToolRequest, input MCPIDInput) (*mcp.CallToolResult, MCPDetailOutput, error) {
	id, err := mcpRequiredID(input.ID)
	if err != nil {
		return nil, MCPDetailOutput{}, err
	}
	loop, err := t.service.Load(id)
	if err != nil {
		return nil, MCPDetailOutput{}, err
	}
	runs, err := t.service.Runs(id, mcpLimit(input.Limit))
	if err != nil {
		return nil, MCPDetailOutput{}, err
	}
	return nil, MCPDetailOutput{Loop: loop, Runs: runs}, nil
}

type MCPCreateInput struct {
	Name            string   `json:"name,omitempty"`
	Prompt          string   `json:"prompt" jsonschema:"task the loop should run"`
	Schedule        Schedule `json:"schedule"`
	Status          string   `json:"status,omitempty" jsonschema:"active or paused; default active"`
	Runtime         string   `json:"runtime,omitempty" jsonschema:"acp or native; default acp"`
	ACPAgent        string   `json:"acp_agent,omitempty" jsonschema:"ACP agent name when runtime is acp; default codex"`
	ModelProvider   string   `json:"model_provider,omitempty"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty" jsonschema:"none, minimal, low, medium, high, xhigh"`
	Directory       string   `json:"directory,omitempty" jsonschema:"workspace-relative or absolute directory for loop runs"`
}

func (t *MCPTools) Create(_ context.Context, _ *mcp.CallToolRequest, input MCPCreateInput) (*mcp.CallToolResult, Loop, error) {
	loop, err := t.service.Create(CreateLoop{
		Name:            input.Name,
		Prompt:          input.Prompt,
		Schedule:        input.Schedule,
		Status:          input.Status,
		Runtime:         input.Runtime,
		ACPAgent:        input.ACPAgent,
		ModelProvider:   input.ModelProvider,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		Directory:       input.Directory,
	})
	return nil, loop, err
}

type MCPUpdateInput struct {
	ID              string    `json:"id" jsonschema:"loop id"`
	Name            *string   `json:"name,omitempty"`
	Prompt          *string   `json:"prompt,omitempty"`
	Schedule        *Schedule `json:"schedule,omitempty"`
	Status          *string   `json:"status,omitempty" jsonschema:"active or paused"`
	Runtime         *string   `json:"runtime,omitempty" jsonschema:"acp or native"`
	ACPAgent        *string   `json:"acp_agent,omitempty"`
	ModelProvider   *string   `json:"model_provider,omitempty"`
	Model           *string   `json:"model,omitempty"`
	ReasoningEffort *string   `json:"reasoning_effort,omitempty" jsonschema:"none, minimal, low, medium, high, xhigh"`
	Directory       *string   `json:"directory,omitempty"`
}

func (t *MCPTools) Update(_ context.Context, _ *mcp.CallToolRequest, input MCPUpdateInput) (*mcp.CallToolResult, Loop, error) {
	id, err := mcpRequiredID(input.ID)
	if err != nil {
		return nil, Loop{}, err
	}
	loop, err := t.service.Update(id, UpdateLoop{
		Name:            input.Name,
		Prompt:          input.Prompt,
		Schedule:        input.Schedule,
		Status:          input.Status,
		Runtime:         input.Runtime,
		ACPAgent:        input.ACPAgent,
		ModelProvider:   input.ModelProvider,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		Directory:       input.Directory,
	})
	return nil, loop, err
}

func (t *MCPTools) Run(ctx context.Context, _ *mcp.CallToolRequest, input MCPIDInput) (*mcp.CallToolResult, Run, error) {
	id, err := mcpRequiredID(input.ID)
	if err != nil {
		return nil, Run{}, err
	}
	run, err := t.service.RunNow(ctx, id)
	return nil, run, err
}

type MCPDeleteOutput struct {
	OK bool `json:"ok"`
}

func (t *MCPTools) Delete(_ context.Context, _ *mcp.CallToolRequest, input MCPIDInput) (*mcp.CallToolResult, MCPDeleteOutput, error) {
	id, err := mcpRequiredID(input.ID)
	if err != nil {
		return nil, MCPDeleteOutput{}, err
	}
	if err := t.service.Delete(id); err != nil {
		return nil, MCPDeleteOutput{}, err
	}
	return nil, MCPDeleteOutput{OK: true}, nil
}

func mcpRequiredID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return "", errors.New("id is required")
	}
	return id, nil
}

func mcpLimit(value int) int {
	if value <= 0 {
		return 20
	}
	if value > 100 {
		return 100
	}
	return value
}
