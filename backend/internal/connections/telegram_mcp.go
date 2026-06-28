package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type TelegramMCPTools struct {
	store  ConnectionToolStore
	sender telegramconnector.Sender
	search telegramconnector.Searcher
}

type TelegramSendMessageInput struct {
	Account   string `json:"account,omitempty" jsonschema:"connected account alias, account id, or connection id; omit only when one Telegram account is connected"`
	Recipient string `json:"recipient" jsonschema:"Telegram username, user:<id>:<access_hash>, chat:<id>, or channel:<id>:<access_hash> recipient"`
	Message   string `json:"message" jsonschema:"message text to send"`
}

type TelegramSendMessageOutput struct {
	Connected       bool                      `json:"connected"`
	AccountRequired bool                      `json:"account_required,omitempty"`
	SenderAvailable bool                      `json:"sender_available"`
	Sent            bool                      `json:"sent"`
	Provider        string                    `json:"provider"`
	Accounts        []integrations.Connection `json:"accounts,omitempty"`
	AccountID       string                    `json:"account_id,omitempty"`
	Alias           string                    `json:"alias,omitempty"`
	Recipient       string                    `json:"recipient,omitempty"`
	MessageID       string                    `json:"message_id,omitempty"`
	PeerID          string                    `json:"peer_id,omitempty"`
	SentAt          time.Time                 `json:"sent_at,omitempty"`
}

type TelegramSearchInput struct {
	Account string `json:"account,omitempty" jsonschema:"connected account alias, account id, or connection id; omit only when one Telegram account is connected"`
	Query   string `json:"query,omitempty" jsonschema:"name, username, phone number, chat title, or Telegram peer recipient to search for; omit to list known contacts and recent dialogs"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum results to return, default 10, max 25"`
}

type TelegramSearchOutput struct {
	Connected         bool                           `json:"connected"`
	AccountRequired   bool                           `json:"account_required,omitempty"`
	SearcherAvailable bool                           `json:"searcher_available"`
	Provider          string                         `json:"provider"`
	Accounts          []integrations.Connection      `json:"accounts,omitempty"`
	AccountID         string                         `json:"account_id,omitempty"`
	Alias             string                         `json:"alias,omitempty"`
	Query             string                         `json:"query,omitempty"`
	Results           []telegramconnector.SearchItem `json:"results,omitempty"`
}

func NewTelegramMCPTools(store ConnectionToolStore, sender telegramconnector.Sender, search telegramconnector.Searcher) *TelegramMCPTools {
	return &TelegramMCPTools{store: store, sender: sender, search: search}
}

func (t *TelegramMCPTools) AddTo(server *mcp.Server) {
	if t.search != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        telegramconnector.ToolSearch,
			Title:       "Search Telegram chats",
			Description: telegramconnector.ToolSearchDescription,
		}, t.SearchTelegram)
	}
	if t.sender != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        telegramconnector.ToolSendMessage,
			Title:       "Send Telegram message",
			Description: telegramconnector.ToolSendMessageDescription,
		}, t.SendTelegramMessage)
	}
}

func (t *TelegramMCPTools) RemoveFrom(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(telegramconnector.ToolSearch, telegramconnector.ToolSendMessage)
	}
}

func (t *TelegramMCPTools) SearchTelegram(ctx context.Context, _ *mcp.CallToolRequest, input TelegramSearchInput) (*mcp.CallToolResult, TelegramSearchOutput, error) {
	query := strings.TrimSpace(input.Query)
	selected, err := selectMCPConnection(ctx, t.store, telegramconnector.ProviderID, telegramconnector.ProviderName, input.Account)
	if err != nil {
		return nil, TelegramSearchOutput{}, err
	}
	out := TelegramSearchOutput{Provider: telegramconnector.ProviderID, Accounts: selected.Connections, Query: query}
	applyTelegramSearchAccount(&out, selected)
	if !selected.Connected || selected.AccountRequired {
		return textResult(selected.Text), out, nil
	}
	if t.search == nil {
		return textResult("Telegram search is not enabled in this runtime."), out, nil
	}
	out.SearcherAvailable = true
	result, err := t.search.Search(ctx, telegramconnector.SearchRequest{
		Connection: selected.Connection,
		Query:      query,
		Limit:      telegramconnector.SearchLimit(input.Limit),
	})
	if err != nil {
		return nil, TelegramSearchOutput{}, err
	}
	out.Results = result.Items
	if len(out.Results) == 0 {
		return textResult(fmt.Sprintf("No Telegram people or chats matched %q.", query)), out, nil
	}
	return textResult(fmt.Sprintf("Found %d Telegram result(s). Use the recipient field with %s.", len(out.Results), telegramconnector.ToolSendMessage)), out, nil
}

func (t *TelegramMCPTools) SendTelegramMessage(ctx context.Context, _ *mcp.CallToolRequest, input TelegramSendMessageInput) (*mcp.CallToolResult, TelegramSendMessageOutput, error) {
	recipient := strings.TrimSpace(input.Recipient)
	if recipient == "" {
		return nil, TelegramSendMessageOutput{}, errors.New("recipient is required")
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return nil, TelegramSendMessageOutput{}, errors.New("message is required")
	}
	selected, err := selectMCPConnection(ctx, t.store, telegramconnector.ProviderID, telegramconnector.ProviderName, input.Account)
	if err != nil {
		return nil, TelegramSendMessageOutput{}, err
	}
	out := TelegramSendMessageOutput{Provider: telegramconnector.ProviderID, Accounts: selected.Connections, Recipient: recipient}
	applyTelegramSendAccount(&out, selected)
	if !selected.Connected || selected.AccountRequired {
		return textResult(selected.Text), out, nil
	}
	if t.sender == nil {
		return textResult("Telegram messaging is not enabled in this runtime."), out, nil
	}
	out.SenderAvailable = true
	result, err := t.sender.SendMessage(ctx, telegramconnector.SendMessageRequest{
		Connection: selected.Connection,
		Recipient:  recipient,
		Message:    message,
	})
	if err != nil {
		return nil, TelegramSendMessageOutput{}, err
	}
	out.Sent = true
	out.MessageID = result.MessageID
	out.PeerID = result.PeerID
	out.SentAt = result.SentAt
	return textResult(fmt.Sprintf("Sent Telegram message to %s.", recipient)), out, nil
}

func applyTelegramSearchAccount(out *TelegramSearchOutput, selected mcpAccountSelection) {
	out.Connected = selected.Connected
	out.AccountRequired = selected.AccountRequired
	out.AccountID = selected.Connection.AccountID
	out.Alias = selected.Connection.Alias
}

func applyTelegramSendAccount(out *TelegramSendMessageOutput, selected mcpAccountSelection) {
	out.Connected = selected.Connected
	out.AccountRequired = selected.AccountRequired
	out.AccountID = selected.Connection.AccountID
	out.Alias = selected.Connection.Alias
}
