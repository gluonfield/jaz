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
	reader telegramconnector.Reader
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

type TelegramReadRecentInput struct {
	Account string `json:"account,omitempty" jsonschema:"connected account alias, account id, or connection id; omit only when one Telegram account is connected"`
	Peer    string `json:"peer" jsonschema:"Telegram recipient or peer returned by telegram_search, such as user:<id>:<access_hash>, chat:<id>, channel:<id>:<access_hash>, or @username"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum recent messages to return, default 50, max 200"`
}

type TelegramReadRecentOutput struct {
	Connected       bool                                  `json:"connected"`
	AccountRequired bool                                  `json:"account_required,omitempty"`
	ReaderAvailable bool                                  `json:"reader_available"`
	Provider        string                                `json:"provider"`
	Accounts        []integrations.Connection             `json:"accounts,omitempty"`
	AccountID       string                                `json:"account_id,omitempty"`
	Alias           string                                `json:"alias,omitempty"`
	PeerID          string                                `json:"peer_id,omitempty"`
	Messages        []telegramconnector.ReadRecentMessage `json:"messages,omitempty"`
}

func NewTelegramMCPTools(store ConnectionToolStore, sender telegramconnector.Sender, search telegramconnector.Searcher, readers ...telegramconnector.Reader) *TelegramMCPTools {
	var reader telegramconnector.Reader
	if len(readers) > 0 {
		reader = readers[0]
	}
	return &TelegramMCPTools{store: store, sender: sender, search: search, reader: reader}
}

func (t *TelegramMCPTools) AddTo(server *mcp.Server) {
	if t.search != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        telegramconnector.ToolSearch,
			Title:       "Search Telegram chats",
			Description: telegramconnector.ToolSearchDescription,
		}, t.SearchTelegram)
	}
	if t.reader != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        telegramconnector.ToolReadRecent,
			Title:       "Read Telegram messages",
			Description: telegramconnector.ToolReadRecentDescription,
		}, t.ReadTelegramRecent)
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
		server.RemoveTools(telegramconnector.ToolSearch, telegramconnector.ToolReadRecent, telegramconnector.ToolSendMessage)
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
	if !connectionHasScope(selected.Connection, connectionScopeContacts) {
		return textResult(mcpScopeDeniedText(telegramconnector.ProviderName, connectionScopeContacts)), out, nil
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

func (t *TelegramMCPTools) ReadTelegramRecent(ctx context.Context, _ *mcp.CallToolRequest, input TelegramReadRecentInput) (*mcp.CallToolResult, TelegramReadRecentOutput, error) {
	peer := strings.TrimSpace(input.Peer)
	if peer == "" {
		return nil, TelegramReadRecentOutput{}, errors.New("peer is required")
	}
	selected, err := selectMCPConnection(ctx, t.store, telegramconnector.ProviderID, telegramconnector.ProviderName, input.Account)
	if err != nil {
		return nil, TelegramReadRecentOutput{}, err
	}
	out := TelegramReadRecentOutput{Provider: telegramconnector.ProviderID, Accounts: selected.Connections}
	applyTelegramReadAccount(&out, selected)
	if !selected.Connected || selected.AccountRequired {
		return textResult(selected.Text), out, nil
	}
	if !connectionHasScope(selected.Connection, connectionScopeMessages) {
		return textResult(mcpScopeDeniedText(telegramconnector.ProviderName, connectionScopeMessages)), out, nil
	}
	if t.reader == nil {
		return textResult("Telegram message reading is not enabled in this runtime."), out, nil
	}
	out.ReaderAvailable = true
	result, err := t.reader.ReadRecent(ctx, telegramconnector.ReadRecentRequest{
		Connection: selected.Connection,
		Peer:       peer,
		Limit:      telegramconnector.ReadRecentLimit(input.Limit),
	})
	if err != nil {
		return nil, TelegramReadRecentOutput{}, err
	}
	out.PeerID = result.PeerID
	out.Messages = result.Messages
	return textResult(fmt.Sprintf("Read %d recent Telegram message(s) from %s.", len(out.Messages), out.PeerID)), out, nil
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
	if !connectionHasScope(selected.Connection, connectionScopeSend) {
		return textResult(mcpScopeDeniedText(telegramconnector.ProviderName, connectionScopeSend)), out, nil
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

func applyTelegramReadAccount(out *TelegramReadRecentOutput, selected mcpAccountSelection) {
	out.Connected = selected.Connected
	out.AccountRequired = selected.AccountRequired
	out.AccountID = selected.Connection.AccountID
	out.Alias = selected.Connection.Alias
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
