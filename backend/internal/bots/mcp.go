package bots

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

type MCPTools struct {
	service *Service
}

func NewMCPTools(service *Service) *MCPTools {
	return &MCPTools{service: service}
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_bots",
		Title:       "List Jaz bots",
		Description: "List the user's Jaz bots and group chats with their ids.",
	}, t.List)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "message_bot",
		Title:       "Message a Jaz bot",
		Description: "Send a message to a Jaz bot or post it to a group chat. Delivery is asynchronous: a bot's reply arrives later as a new turn in this thread, so do not wait for it.",
	}, t.Message)
}

// AddBotTo adds the tools only a bot's own thread gets.
func (t *MCPTools) AddBotTo(server *mcp.Server) {
	updateSchema, err := jsonschema.For[MCPTaskUpdateInput](nil)
	if err != nil {
		panic(err)
	}
	updateSchema.Properties["status"].Enum = []any{storage.CommitmentPending, storage.CommitmentInProgress, storage.CommitmentWaitingForUser, storage.CommitmentStopped, storage.CommitmentDone, storage.CommitmentDropped}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_message",
		Title:       "Send a chat message",
		Description: "Send a text message as yourself. In a group chat turn it posts to the group, when answering a bot it goes to that bot, and otherwise it reaches the user in your chat. App presentation tools can also deliver a visible answer; do not repeat a result the host has already presented.",
	}, t.Send)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bot_task_create",
		Title:       "Create a task",
		Description: "Record unfinished work the bot must complete beyond this turn. State the requested outcome and a concrete check for when it is done, not just a work label. Create a task before delegating work or promising a later result. Returns its id; pass it to create_thread as task. The task is owed to whoever this turn answers.",
	}, t.CreateTask)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bot_task_update",
		InputSchema: updateSchema,
		Title:       "Update a task",
		Description: "Update a task's progress, status or prerequisites. Use pending, in_progress, waiting_for_user, stopped, completed or cancelled. Before completed, check the result against the requested outcome and state how it meets the completion check in message; a finished worker/thread alone is not completion. If only partial, keep it in_progress and state what remains in next; use waiting_for_user for a missing answer or stopped when no useful path remains. stopped requires message explaining attempts, the obstacle and what could justify retrying, or honestly no known retry condition. Stop known owned outside jobs through their owner first; cancelling a thread does not establish they stopped. Recorded foreground/background work must stop first. Resume stopped work only to pending with message justifying relevant new user direction or changed conditions. Unrelated messages and stale results never resume it. completed delivers message as the result and closes the task together; cancelled requires message explaining why. waiting_for_user delivers message as an actual question and records its reference. Resume only after a relevant user answer, setting pending or in_progress with answered_question equal to the current question_seq. Use check_at for a future check without repeated wakes. Dependencies stay pending until their prerequisites end.",
	}, t.UpdateTask)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bot_task_list",
		Title:       "List unfinished tasks",
		Description: "Read all of your unfinished tasks, including pending, in_progress, waiting_for_user and stopped, with their ids, prerequisites, question references and current worker states. Includes every open task without a result limit.",
	}, t.ListTasks)
}

type MCPListInput struct{}

type MCPBot struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Members []string `json:"members,omitempty"`
}

type MCPListOutput struct {
	Bots []MCPBot `json:"bots"`
}

func (t *MCPTools) List(context.Context, *mcp.CallToolRequest, MCPListInput) (*mcp.CallToolResult, MCPListOutput, error) {
	bots, err := t.service.List()
	if err != nil {
		return nil, MCPListOutput{}, err
	}
	out := MCPListOutput{Bots: make([]MCPBot, 0, len(bots))}
	for _, bot := range bots {
		out.Bots = append(out.Bots, MCPBot{ID: bot.ID, Kind: bot.Kind, Name: bot.Name, Members: bot.Members})
	}
	return nil, out, nil
}

type MCPMessageInput struct {
	Bot     string  `json:"bot" jsonschema:"bot or group id, a bot:<id> mention target, or the exact name"`
	Message string  `json:"message"`
	Done    []int64 `json:"done,omitempty" jsonschema:"Your task ids whose requested outcomes this message actually delivers to this bot; a finished worker alone is not enough. It goes to the thread that asked and the tasks close as it arrives."`
}

type MCPMessageOutput struct {
	Delivered bool `json:"delivered"`
}

func (t *MCPTools) Message(_ context.Context, req *mcp.CallToolRequest, input MCPMessageInput) (*mcp.CallToolResult, MCPMessageOutput, error) {
	if err := t.service.Message(mcpsession.SessionID(req), input.Bot, input.Message, input.Done...); err != nil {
		return nil, MCPMessageOutput{}, err
	}
	return nil, MCPMessageOutput{Delivered: true}, nil
}

type MCPSendInput struct {
	Message string  `json:"message"`
	Done    []int64 `json:"done,omitempty" jsonschema:"Your task ids whose requested outcomes this message actually delivers; a finished worker alone is not enough. They close as it reaches whoever they are owed to. bot_task_update with status completed also delivers and closes a task."`
}

type MCPSendOutput struct {
	Sent bool `json:"sent"`
}

func (t *MCPTools) Send(_ context.Context, req *mcp.CallToolRequest, input MCPSendInput) (*mcp.CallToolResult, MCPSendOutput, error) {
	if err := t.service.Say(mcpsession.SessionID(req), input.Message, input.Done...); err != nil {
		return nil, MCPSendOutput{}, err
	}
	return nil, MCPSendOutput{Sent: true}, nil
}

type MCPTaskCreateInput struct {
	Name    string `json:"name,omitempty" jsonschema:"Compact task name for headings and lists. Keep the full outcome and completion check in request."`
	Request string `json:"request" jsonschema:"Requested outcome and concrete check for when it is done, not just a work label. Context belongs in the linked work threads."`
	Next    string `json:"next,omitempty" jsonschema:"The next step on this task."`
}

type MCPTaskUpdateInput struct {
	ID               int64    `json:"id" jsonschema:"The open task's id."`
	Name             string   `json:"name,omitempty" jsonschema:"Compact task name for headings and lists; omit to preserve. Keep the full outcome and completion check in request."`
	Request          string   `json:"request,omitempty" jsonschema:"Requested outcome and concrete check for when it is done; change only if the requested outcome changes. Context belongs in the linked work threads."`
	Next             string   `json:"next,omitempty" jsonschema:"Progress or the next step."`
	Status           string   `json:"status,omitempty" jsonschema:"Task status; omit to preserve it."`
	Message          string   `json:"message,omitempty" jsonschema:"For completed, the result and how it meets the requested outcome; for waiting_for_user, the question to deliver; for stopped or cancelled, the reason; for stopped to pending, the justified resume decision. Stopping, resuming, completion or cancellation takes only id, status and message."`
	AnsweredQuestion int64    `json:"answered_question,omitempty" jsonschema:"Current question_seq whose user reply answers this task; required to resume waiting_for_user."`
	CheckAt          *string  `json:"check_at,omitempty" jsonschema:"Next check time in RFC3339 for active work. Omit to preserve; empty string clears it. Work stays Active and the watchdog waits until this time."`
	DependsOn        *[]int64 `json:"depends_on,omitempty" jsonschema:"Ids of this bot's prerequisite tasks. Omit to preserve; send an empty list to clear. Keep dependent work pending until prerequisites end."`
}

type MCPTaskOutput struct {
	ID int64 `json:"id"`
}

func (t *MCPTools) CreateTask(_ context.Context, req *mcp.CallToolRequest, input MCPTaskCreateInput) (*mcp.CallToolResult, MCPTaskOutput, error) {
	id, err := t.service.Commit(mcpsession.SessionID(req), Commit{Name: input.Name, Request: input.Request, Next: input.Next})
	return nil, MCPTaskOutput{ID: id}, err
}

func (t *MCPTools) UpdateTask(_ context.Context, req *mcp.CallToolRequest, input MCPTaskUpdateInput) (*mcp.CallToolResult, MCPTaskOutput, error) {
	if input.ID <= 0 {
		return nil, MCPTaskOutput{}, errors.New("task id must be positive")
	}
	var checkAt *time.Time
	if input.CheckAt != nil {
		at := time.Time{}
		if value := strings.TrimSpace(*input.CheckAt); value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return nil, MCPTaskOutput{}, errors.New("check_at must be an RFC3339 timestamp or empty to clear")
			}
			at = parsed
		}
		checkAt = &at
	}
	id, err := t.service.Commit(mcpsession.SessionID(req), Commit{ID: input.ID, Name: input.Name, Request: input.Request, Next: input.Next, Status: input.Status, Message: input.Message, DependsOn: input.DependsOn, AnsweredQuestion: input.AnsweredQuestion, CheckAt: checkAt})
	return nil, MCPTaskOutput{ID: id}, err
}

type MCPTasksOutput struct {
	Tasks []Owed `json:"tasks"`
}

func (t *MCPTools) ListTasks(_ context.Context, req *mcp.CallToolRequest, _ MCPListInput) (*mcp.CallToolResult, MCPTasksOutput, error) {
	tasks, err := t.service.Tasks(mcpsession.SessionID(req))
	return nil, MCPTasksOutput{Tasks: tasks}, err
}
