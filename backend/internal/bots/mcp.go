package bots

import (
	"context"

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
		Name:        "commitment",
		Title:       "Keep a commitment",
		Description: "Keep the list of what you owe beyond this turn: work you hand to threads and answers you owe once something arrives. Open one with request before you start a thread for it; it is owed to whoever this turn answers. Set next as it moves on, and waiting while it waits on something other than its threads. It closes as done when you deliver its result with send_message or message_bot and its number in done; drop it, saying why, when it is no longer wanted or was delivered without done.",
	}, t.Commit)
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
	Done    []int64 `json:"done,omitempty" jsonschema:"Your commitments to this bot that the message delivers; it goes to the thread of the bot that asked and they close as it arrives."`
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
	Done    []int64 `json:"done,omitempty" jsonschema:"Your commitments whose result this message delivers; they close as it reaches whoever they are owed to."`
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

type MCPCommitmentInput struct {
	ID      int64  `json:"id,omitempty" jsonschema:"The commitment's number; omit to open one."`
	Request string `json:"request,omitempty" jsonschema:"What was asked, in a few words."`
	Next    string `json:"next,omitempty" jsonschema:"Your next step on it."`
	Waiting string `json:"waiting,omitempty" jsonschema:"What it waits on besides its threads: requester, an answer only whoever asked can give, which you have asked for; outside, a job outside Jaz that cannot tell you when it ends; none once it moves again. Omit to leave it as it is. Starting a thread for it sets none."`
	Drop    string `json:"drop,omitempty" jsonschema:"Why it closes without delivering: cancelled, superseded or delivered earlier."`
}

type MCPCommitmentOutput struct {
	ID int64 `json:"id"`
}

func (t *MCPTools) Commit(_ context.Context, req *mcp.CallToolRequest, input MCPCommitmentInput) (*mcp.CallToolResult, MCPCommitmentOutput, error) {
	id, err := t.service.Commit(mcpsession.SessionID(req), Commit(input))
	return nil, MCPCommitmentOutput{ID: id}, err
}
