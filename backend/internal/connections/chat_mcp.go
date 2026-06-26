package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type ChatToolStore interface {
	ListConnections(context.Context, string) ([]integrations.Connection, error)
}

type ChatSender interface {
	ProviderID() string
	SendMessage(context.Context, ChatSendRequest) (ChatSendResult, error)
}

type ChatSendRequest struct {
	Connection integrations.Connection
	Recipient  string
	Message    string
}

type ChatSendResult struct {
	MessageID      string    `json:"message_id,omitempty"`
	ConversationID string    `json:"conversation_id,omitempty"`
	SentAt         time.Time `json:"sent_at,omitempty"`
}

type ChatMCPTools struct {
	store   ChatToolStore
	senders map[string]ChatSender
}

type ChatSendMessageInput struct {
	Account   string `json:"account,omitempty" jsonschema:"connected account alias, account id, or connection id; omit only when one account is connected for this provider"`
	Recipient string `json:"recipient" jsonschema:"phone number, username, provider user id, or conversation id to send to"`
	Message   string `json:"message" jsonschema:"message text to send"`
}

type ChatSendMessageOutput struct {
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
	ConversationID  string                    `json:"conversation_id,omitempty"`
	SentAt          time.Time                 `json:"sent_at,omitempty"`
}

func NewChatMCPTools(store ChatToolStore, senders ...ChatSender) *ChatMCPTools {
	tools := &ChatMCPTools{store: store, senders: map[string]ChatSender{}}
	for _, sender := range senders {
		if sender == nil {
			continue
		}
		tools.senders[sender.ProviderID()] = sender
	}
	return tools
}

func (t *ChatMCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        whatsapp.ToolSendMessage,
		Title:       "Send WhatsApp message",
		Description: "Send a WhatsApp message from one connected account to a phone number, contact id, or conversation id. Requires a configured WhatsApp sender adapter.",
	}, t.SendWhatsAppMessage)
	mcp.AddTool(server, &mcp.Tool{
		Name:        telegram.ToolSendMessage,
		Title:       "Send Telegram message",
		Description: "Send a Telegram message from one connected account to a username, user id, or chat id. Requires a configured Telegram sender adapter.",
	}, t.SendTelegramMessage)
}

func (t *ChatMCPTools) RemoveFrom(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(whatsapp.ToolSendMessage, telegram.ToolSendMessage)
	}
}

func (t *ChatMCPTools) SendWhatsAppMessage(ctx context.Context, _ *mcp.CallToolRequest, input ChatSendMessageInput) (*mcp.CallToolResult, ChatSendMessageOutput, error) {
	return t.sendMessage(ctx, whatsapp.ProviderID, whatsapp.ProviderName, input)
}

func (t *ChatMCPTools) SendTelegramMessage(ctx context.Context, _ *mcp.CallToolRequest, input ChatSendMessageInput) (*mcp.CallToolResult, ChatSendMessageOutput, error) {
	return t.sendMessage(ctx, telegram.ProviderID, telegram.ProviderName, input)
}

func (t *ChatMCPTools) sendMessage(ctx context.Context, provider, providerName string, input ChatSendMessageInput) (*mcp.CallToolResult, ChatSendMessageOutput, error) {
	recipient := strings.TrimSpace(input.Recipient)
	if recipient == "" {
		return nil, ChatSendMessageOutput{}, errors.New("recipient is required")
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return nil, ChatSendMessageOutput{}, errors.New("message is required")
	}
	connections, err := t.store.ListConnections(ctx, provider)
	if err != nil {
		return nil, ChatSendMessageOutput{}, err
	}
	out := ChatSendMessageOutput{Provider: provider, Accounts: connections, Recipient: recipient}
	connection, ok := selectConnection(connections, input.Account)
	if !ok {
		if len(connections) > 1 {
			out.Connected = true
			out.AccountRequired = true
			return textResult(chatAccountRequiredText(providerName, connections)), out, nil
		}
		return textResult(providerName + " is not connected. Connect it in Settings > Connections."), out, nil
	}
	out.Connected = true
	out.AccountID = connection.AccountID
	out.Alias = connection.Alias
	sender := t.senders[provider]
	if sender == nil {
		return textResult(providerName + " messaging is not enabled yet. A provider sender adapter is required before Jaz can send messages."), out, nil
	}
	out.SenderAvailable = true
	result, err := sender.SendMessage(ctx, ChatSendRequest{
		Connection: connection,
		Recipient:  recipient,
		Message:    message,
	})
	if err != nil {
		return nil, ChatSendMessageOutput{}, err
	}
	out.Sent = true
	out.MessageID = result.MessageID
	out.ConversationID = result.ConversationID
	out.SentAt = result.SentAt
	return textResult(fmt.Sprintf("Sent %s message to %s.", providerName, recipient)), out, nil
}

func chatAccountRequiredText(providerName string, connections []integrations.Connection) string {
	var refs []string
	for _, connection := range connections {
		if ref := connection.AccountRef(); ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return "Multiple " + providerName + " accounts are connected. Specify the account alias, account id, or connection id."
	}
	return "Multiple " + providerName + " accounts are connected. Specify account as one of: " + strings.Join(refs, ", ") + "."
}
