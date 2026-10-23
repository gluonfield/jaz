package bots

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_message",
		Title:       "Send a chat message",
		Description: "Send a text message as yourself. In a group chat turn it posts to the group, when answering a bot it goes to that bot, and otherwise it reaches the user in your chat. App presentation tools can also deliver a visible answer; do not repeat a result the host has already presented.",
	}, t.Send)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bot_task_create",
		Title:       "Create a task",
		Description: "Record unfinished work the bot must complete beyond this turn. Create a task before delegating work or promising a later result. Returns its id; pass it to create_thread as task. The task is owed to whoever this turn answers.",
	}, t.CreateTask)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bot_task_update",
		Title:       "Update a task",
		Description: "Update an open task's request, next step or waiting state. Use for progress and follow-up; the task stays open until its result is delivered or it is cancelled.",
	}, t.UpdateTask)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bot_task_complete",
		Title:       "Complete tasks",
		Description: "Deliver completed task results and close the tasks as the message reaches whoever this turn answers. A progress update or question leaves tasks open: use send_message. To deliver to another bot from a different turn, use message_bot with done.",
	}, t.CompleteTasks)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bot_task_cancel",
		Title:       "Cancel a task",
		Description: "Cancel an open task with a reason when it is no longer wanted, superseded or already delivered. Stop its working threads first. Completion with a new result uses bot_task_complete.",
	}, t.CancelTask)
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
	Done    []int64 `json:"done,omitempty" jsonschema:"Your task ids whose results this message delivers to this bot; it goes to the thread that asked and the tasks close as it arrives."`
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
	Done    []int64 `json:"done,omitempty" jsonschema:"Your task ids whose results this message delivers; they close as it reaches whoever they are owed to. bot_task_complete also delivers and closes tasks."`
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
	Request string `json:"request" jsonschema:"What the bot must finish, in a few words."`
	Next    string `json:"next,omitempty" jsonschema:"The next step on this task."`
}

type MCPTaskUpdateInput struct {
	ID      int64  `json:"id" jsonschema:"The open task's id."`
	Request string `json:"request,omitempty" jsonschema:"What was asked, in a few words."`
	Next    string `json:"next,omitempty" jsonschema:"Your next step on it."`
	Waiting string `json:"waiting,omitempty" jsonschema:"What it waits on besides its threads: requester, an answer only whoever asked can give, which you have asked for; outside, a job outside Jaz that cannot tell you when it ends; none once it moves again. Omit to leave it as it is. Starting a thread for it sets none."`
}

type MCPTaskOutput struct {
	ID int64 `json:"id"`
}

func (t *MCPTools) CreateTask(_ context.Context, req *mcp.CallToolRequest, input MCPTaskCreateInput) (*mcp.CallToolResult, MCPTaskOutput, error) {
	id, err := t.service.Commit(mcpsession.SessionID(req), Commit{Request: input.Request, Next: input.Next})
	return nil, MCPTaskOutput{ID: id}, err
}

func (t *MCPTools) UpdateTask(_ context.Context, req *mcp.CallToolRequest, input MCPTaskUpdateInput) (*mcp.CallToolResult, MCPTaskOutput, error) {
	if input.ID <= 0 {
		return nil, MCPTaskOutput{}, errors.New("task id must be positive")
	}
	id, err := t.service.Commit(mcpsession.SessionID(req), Commit{ID: input.ID, Request: input.Request, Next: input.Next, Waiting: input.Waiting})
	return nil, MCPTaskOutput{ID: id}, err
}

type MCPTaskCompleteInput struct {
	IDs     []int64 `json:"ids" jsonschema:"The task ids whose completed results this message delivers."`
	Message string  `json:"message" jsonschema:"The completed results for whoever this turn answers."`
}

func (t *MCPTools) CompleteTasks(_ context.Context, req *mcp.CallToolRequest, input MCPTaskCompleteInput) (*mcp.CallToolResult, MCPSendOutput, error) {
	if len(input.IDs) == 0 {
		return nil, MCPSendOutput{}, errors.New("task ids are required")
	}
	if err := t.service.Say(mcpsession.SessionID(req), input.Message, input.IDs...); err != nil {
		return nil, MCPSendOutput{}, err
	}
	return nil, MCPSendOutput{Sent: true}, nil
}

type MCPTaskCancelInput struct {
	ID     int64  `json:"id" jsonschema:"The open task's id."`
	Reason string `json:"reason" jsonschema:"Why the task is cancelled, superseded or already delivered."`
}

func (t *MCPTools) CancelTask(_ context.Context, req *mcp.CallToolRequest, input MCPTaskCancelInput) (*mcp.CallToolResult, MCPTaskOutput, error) {
	if input.ID <= 0 || strings.TrimSpace(input.Reason) == "" {
		return nil, MCPTaskOutput{}, errors.New("a positive task id and cancellation reason are required")
	}
	id, err := t.service.Commit(mcpsession.SessionID(req), Commit{ID: input.ID, Drop: input.Reason})
	return nil, MCPTaskOutput{ID: id}, err
}
