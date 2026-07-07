package loops

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
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
	// boards, when set, lets create/update assign the loop's widget to boards
	// and exposes loop_boards. Nil leaves loops board-less.
	boards BoardService
	// agentNames lists the currently enabled ACP agent names for the schema
	// hint. Nil omits the hint (the manager still resolves the default).
	agentNames func() []string
	// card, when its Store is set, emits a loop_created card into the calling
	// thread after a successful create.
	card CardSink
}

// MCPOption configures optional MCPTools dependencies.
type MCPOption func(*MCPTools)

// WithBoards enables board assignment and the loop_boards listing tool.
func WithBoards(boards BoardService) MCPOption {
	return func(t *MCPTools) { t.boards = boards }
}

// WithAgentNames supplies the enabled-agent lister used to build the acp_agent
// hint shown on loop_create/loop_update.
func WithAgentNames(names func() []string) MCPOption {
	return func(t *MCPTools) { t.agentNames = names }
}

// WithEvents emits a loop_created card into the calling thread on a successful
// create, persisted and streamed through the session-events bus.
func WithEvents(store SessionEventAppender, bus SessionEventPublisher) MCPOption {
	return func(t *MCPTools) { t.card = CardSink{Store: store, Bus: bus} }
}

func NewMCPTools(service MCPService, opts ...MCPOption) *MCPTools {
	t := &MCPTools{service: service}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	agentHint := ""
	if agents := t.availableAgents(); len(agents) > 0 {
		agentHint = " Set acp_agent to one of: " + strings.Join(agents, ", ") + "; omit it to use the workspace default agent."
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_list",
		Title:       "List Jaz loops",
		Description: "List active Jaz loops.",
	}, t.List)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_get",
		Title:       "Get Jaz loop",
		Description: "Get one Jaz loop, its recent runs, and the boards its widget is on.",
	}, t.Get)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_create",
		Title:       "Create Jaz loop",
		Description: "Create a scheduled Jaz loop." + agentHint,
	}, t.Create)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_update",
		Title:       "Update Jaz loop",
		Description: "Update an existing Jaz loop." + agentHint,
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
	if t.boards != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "loop_boards",
			Title:       "List Jaz boards",
			Description: "List Jaz boards a loop's widget can be assigned to via board_ids on loop_create/loop_update.",
		}, t.Boards)
	}
}

// availableAgents returns the enabled ACP agent names, or nil when no lister is wired.
func (t *MCPTools) availableAgents() []string {
	if t.agentNames == nil {
		return nil
	}
	return t.agentNames()
}

// coordinator is the shared create/update-with-boards use case; loop_create and
// loop_update are thin adapters over it.
func (t *MCPTools) coordinator() Coordinator {
	return Coordinator{
		Loops:  t.service,
		Boards: t.boards,
		Card:   t.card,
	}
}

func (t *MCPTools) boardsForLoop(loopID string) []string {
	if t.boards == nil {
		return []string{}
	}
	ids, err := t.boards.BoardsForLoop(loopID)
	if err != nil || ids == nil {
		return []string{}
	}
	return ids
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
	Loop     Loop     `json:"loop"`
	Runs     []Run    `json:"runs,omitempty"`
	BoardIDs []string `json:"board_ids"`
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
	return nil, MCPDetailOutput{Loop: loop, Runs: runs, BoardIDs: t.boardsForLoop(id)}, nil
}

type MCPBoardsOutput struct {
	Boards []BoardSummary `json:"boards"`
}

func (t *MCPTools) Boards(context.Context, *mcp.CallToolRequest, MCPListInput) (*mcp.CallToolResult, MCPBoardsOutput, error) {
	if t.boards == nil {
		return nil, MCPBoardsOutput{Boards: []BoardSummary{}}, nil
	}
	boards, err := t.boards.ListBoards()
	if err != nil {
		return nil, MCPBoardsOutput{}, err
	}
	if boards == nil {
		boards = []BoardSummary{}
	}
	return nil, MCPBoardsOutput{Boards: boards}, nil
}

type MCPCreateInput struct {
	Name            string   `json:"name,omitempty"`
	Prompt          string   `json:"prompt" jsonschema:"task the loop should run"`
	Schedule        Schedule `json:"schedule"`
	Status          string   `json:"status,omitempty" jsonschema:"active or paused; default active"`
	Runtime         string   `json:"runtime,omitempty" jsonschema:"acp; default acp"`
	ACPAgent        string   `json:"acp_agent,omitempty" jsonschema:"ACP agent that runs the loop; see this tool's description for the available agents. Omit to use the workspace default agent."`
	ModelProvider   string   `json:"model_provider,omitempty"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty" jsonschema:"none, minimal, low, medium, high, xhigh, max"`
	Directory       string   `json:"directory,omitempty" jsonschema:"workspace-relative or absolute directory for loop runs"`
	BoardIDs        []string `json:"board_ids,omitempty" jsonschema:"board ids to place this loop's widget on; assignment is what enables the widget. Use loop_boards to list ids."`
}

func (t *MCPTools) Create(_ context.Context, req *mcp.CallToolRequest, input MCPCreateInput) (*mcp.CallToolResult, Loop, error) {
	loop, err := t.coordinator().Create(CreateLoop{
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
	}, input.BoardIDs, mcpsession.SessionID(req))
	return nil, loop, err
}

type MCPUpdateInput struct {
	ID              string    `json:"id" jsonschema:"loop id"`
	Name            *string   `json:"name,omitempty"`
	Prompt          *string   `json:"prompt,omitempty"`
	Schedule        *Schedule `json:"schedule,omitempty"`
	Status          *string   `json:"status,omitempty" jsonschema:"active or paused"`
	Runtime         *string   `json:"runtime,omitempty" jsonschema:"acp"`
	ACPAgent        *string   `json:"acp_agent,omitempty" jsonschema:"ACP agent that runs the loop; see this tool's description for the available agents."`
	ModelProvider   *string   `json:"model_provider,omitempty"`
	Model           *string   `json:"model,omitempty"`
	ReasoningEffort *string   `json:"reasoning_effort,omitempty" jsonschema:"none, minimal, low, medium, high, xhigh, max"`
	Directory       *string   `json:"directory,omitempty"`
	// BoardIDs reassigns the loop's widget to exactly these boards. Omit to
	// leave assignments untouched; pass an empty array to clear them.
	BoardIDs *[]string `json:"board_ids,omitempty" jsonschema:"boards to place this loop's widget on; omit to leave unchanged, empty array to remove from all boards. Use loop_boards to list ids."`
}

func (t *MCPTools) Update(_ context.Context, _ *mcp.CallToolRequest, input MCPUpdateInput) (*mcp.CallToolResult, Loop, error) {
	id, err := mcpRequiredID(input.ID)
	if err != nil {
		return nil, Loop{}, err
	}
	loop, err := t.coordinator().Update(id, UpdateLoop{
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
	}, input.BoardIDs)
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
