package loops

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	card  CardSink
	owner OwnerFunc
	scope func(threadID string) string
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

// WithOwner assigns every created loop to a bot; see Coordinator.Owner.
func WithOwner(owner OwnerFunc) MCPOption {
	return func(t *MCPTools) { t.owner = owner }
}

// WithScope limits a bot's threads to the bot's own routines. scope returns the
// bot a thread belongs to, or "" for a thread that sees every routine.
func WithScope(scope func(threadID string) string) MCPOption {
	return func(t *MCPTools) { t.scope = scope }
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
		Description: "List routines. A routine is a scheduled or event-triggered prompt owned by a Jaz bot; bot_id names the owner. Called from a bot's thread, it lists only that bot's routines.",
	}, t.List)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_get",
		Title:       "Get Jaz loop",
		Description: "Get one Jaz loop, its recent runs, and the boards its widget is on.",
	}, t.Get)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_create",
		Title:       "Create Jaz loop",
		Description: "Create a routine: a prompt a Jaz bot runs on a schedule or when a trigger fires. Called from a bot's thread, the routine belongs to that bot; otherwise pass bot, or a new bot named after the routine is created (acp_agent and model configure that new bot)." + agentHint,
	}, t.Create)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_update",
		Title:       "Update Jaz loop",
		Description: "Update an existing Jaz loop." + agentHint,
	}, t.Update)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "loop_run",
		Title:       "Run Jaz loop now",
		Description: "Queue a manual run for a Jaz loop. Returns before the work finishes. A bot-owned routine runs in a separate turn and delivers its own outcome; after starting it, leave the work and answer to that turn.",
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
		Owner:  t.owner,
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

func (t *MCPTools) List(_ context.Context, req *mcp.CallToolRequest, _ MCPListInput) (*mcp.CallToolResult, MCPListOutput, error) {
	items, err := t.service.List()
	if bot := t.bot(req); bot != "" {
		items = slices.DeleteFunc(items, func(loop Loop) bool { return loop.BotID != bot })
	}
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

func (t *MCPTools) Get(_ context.Context, req *mcp.CallToolRequest, input MCPIDInput) (*mcp.CallToolResult, MCPDetailOutput, error) {
	loop, err := t.load(req, input.ID)
	if err != nil {
		return nil, MCPDetailOutput{}, err
	}
	runs, err := t.service.Runs(loop.ID, mcpLimit(input.Limit))
	if err != nil {
		return nil, MCPDetailOutput{}, err
	}
	return nil, MCPDetailOutput{Loop: loop, Runs: runs, BoardIDs: t.boardsForLoop(loop.ID)}, nil
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
	Bot             string   `json:"bot,omitempty" jsonschema:"id of the bot that owns the routine; defaults to the calling bot. A routine on a board belongs to the board"`
	Trigger         *Trigger `json:"trigger,omitempty" jsonschema:"fire on an event instead of the schedule: kind gmail, whatsapp, telegram, slack or webhook, with optional from, subject (email subject or Slack #channel) and contains filters"`
	WhenMissed      string   `json:"when_missed,omitempty" jsonschema:"what to do with runs and events missed while Jaz was not running: skip (default) drops them; catch_up covers them all in one run when Jaz is back"`
	Thread          string   `json:"thread,omitempty" jsonschema:"where a bot's routine runs: child (default), a fresh thread of its own whose reply reaches the bot when it has something for the user; main, a turn in the bot's own chat, for runs that need that conversation"`
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
		BotID:           strings.TrimSpace(input.Bot),
		Trigger:         input.Trigger,
		WhenMissed:      input.WhenMissed,
		Thread:          input.Thread,
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
	Trigger         *Trigger  `json:"trigger,omitempty" jsonschema:"switch the routine to an event trigger; setting schedule switches it back"`
	WhenMissed      *string   `json:"when_missed,omitempty" jsonschema:"skip or catch_up; see loop_create"`
	Thread          *string   `json:"thread,omitempty" jsonschema:"child or main; see loop_create"`
	// BoardIDs reassigns the loop's widget to exactly these boards. Omit to
	// leave assignments untouched; pass an empty array to clear them.
	BoardIDs *[]string `json:"board_ids,omitempty" jsonschema:"boards to place this loop's widget on; omit to leave unchanged, empty array to remove from all boards. Use loop_boards to list ids."`
}

func (t *MCPTools) Update(_ context.Context, req *mcp.CallToolRequest, input MCPUpdateInput) (*mcp.CallToolResult, Loop, error) {
	current, err := t.load(req, input.ID)
	if err != nil {
		return nil, Loop{}, err
	}
	loop, err := t.coordinator().Update(current.ID, UpdateLoop{
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
		Trigger:         input.Trigger,
		WhenMissed:      input.WhenMissed,
		Thread:          input.Thread,
	}, input.BoardIDs)
	return nil, loop, err
}

func (t *MCPTools) Run(ctx context.Context, req *mcp.CallToolRequest, input MCPIDInput) (*mcp.CallToolResult, Run, error) {
	loop, err := t.load(req, input.ID)
	if err != nil {
		return nil, Run{}, err
	}
	run, err := t.service.RunNow(ctx, loop.ID)
	if err != nil {
		return nil, run, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Run queued. The routine will do the work in its own turn; do not repeat that work or its answer in this turn."}}}, run, nil
}

type MCPDeleteOutput struct {
	OK bool `json:"ok"`
}

func (t *MCPTools) Delete(_ context.Context, req *mcp.CallToolRequest, input MCPIDInput) (*mcp.CallToolResult, MCPDeleteOutput, error) {
	loop, err := t.load(req, input.ID)
	if err != nil {
		return nil, MCPDeleteOutput{}, err
	}
	if err := t.service.Delete(loop.ID); err != nil {
		return nil, MCPDeleteOutput{}, err
	}
	return nil, MCPDeleteOutput{OK: true}, nil
}

// bot returns the bot whose routines the caller is limited to, or "" for all.
func (t *MCPTools) bot(req *mcp.CallToolRequest) string {
	if t.scope == nil {
		return ""
	}
	return t.scope(mcpsession.SessionID(req))
}

// load returns the loop the caller names, provided the caller may see it.
func (t *MCPTools) load(req *mcp.CallToolRequest, value string) (Loop, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return Loop{}, errors.New("id is required")
	}
	loop, err := t.service.Load(id)
	if err != nil {
		return Loop{}, err
	}
	if bot := t.bot(req); bot != "" && loop.BotID != bot {
		return Loop{}, fmt.Errorf("loop %s is not one of your routines", id)
	}
	return loop, nil
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
