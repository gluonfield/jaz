package sessiongoal

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

const (
	MCPToolCreate = "create_goal"
	MCPToolGet    = "get_goal"
	MCPToolUpdate = "update_goal"
)

type MCPTools struct {
	service *Service
}

type GetInput struct{}

type GoalOutput struct {
	Goal *goal.PublicState `json:"goal,omitempty"`
}

func NewMCPTools(service *Service) *MCPTools {
	return &MCPTools{service: service}
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolCreate,
		Title:       "Create goal",
		Description: "Create the active Jaz goal for this thread. Call this once at the start of Goal mode with the concise objective you will pursue.",
	}, t.Create)
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolGet,
		Title:       "Get goal",
		Description: "Return the active Jaz goal with tokens_used, token_budget, remaining_tokens, and time_used_seconds computed by Jaz.",
	}, t.Get)
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolUpdate,
		Title:       "Update goal",
		Description: "Update the active Jaz goal status. Use status active while continuing, complete only when the objective is achieved, or blocked when progress cannot continue without user input or an external change.",
	}, t.Update)
}

func (t *MCPTools) Create(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, GoalOutput, error) {
	state, err := t.service.Create(ctx, mcpsession.SessionID(req), input)
	return nil, GoalOutput{Goal: goal.PublicStateFrom(state)}, err
}

func (t *MCPTools) Get(ctx context.Context, req *mcp.CallToolRequest, _ GetInput) (*mcp.CallToolResult, GoalOutput, error) {
	state, err := t.service.Get(ctx, mcpsession.SessionID(req))
	return nil, GoalOutput{Goal: goal.PublicStateFrom(state)}, err
}

func (t *MCPTools) Update(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, GoalOutput, error) {
	state, err := t.service.Update(ctx, mcpsession.SessionID(req), input)
	return nil, GoalOutput{Goal: goal.PublicStateFrom(state)}, err
}
