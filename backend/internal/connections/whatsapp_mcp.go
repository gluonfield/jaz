package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type WhatsAppMCPTools struct {
	store  ConnectionToolStore
	sender whatsappconnector.Sender
	search whatsappconnector.Searcher
}

type WhatsAppSendMessageInput struct {
	Account   string `json:"account,omitempty" jsonschema:"connected account alias, account id, or connection id; omit only when one WhatsApp account is connected"`
	Recipient string `json:"recipient" jsonschema:"phone number or WhatsApp JID to send to"`
	Message   string `json:"message" jsonschema:"message text to send"`
}

type WhatsAppSendMessageOutput struct {
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
	JID             string                    `json:"jid,omitempty"`
	SentAt          time.Time                 `json:"sent_at,omitempty"`
}

type WhatsAppSearchInput struct {
	Account string `json:"account,omitempty" jsonschema:"connected account alias, account id, or connection id; omit only when one WhatsApp account is connected"`
	Query   string `json:"query,omitempty" jsonschema:"name, phone number, or WhatsApp JID to search for; omit to list known contacts"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum results to return, default 10, max 25"`
}

type WhatsAppSearchOutput struct {
	Connected         bool                           `json:"connected"`
	AccountRequired   bool                           `json:"account_required,omitempty"`
	SearcherAvailable bool                           `json:"searcher_available"`
	Provider          string                         `json:"provider"`
	Accounts          []integrations.Connection      `json:"accounts,omitempty"`
	AccountID         string                         `json:"account_id,omitempty"`
	Alias             string                         `json:"alias,omitempty"`
	Query             string                         `json:"query,omitempty"`
	Results           []whatsappconnector.SearchItem `json:"results,omitempty"`
}

func NewWhatsAppMCPTools(store ConnectionToolStore, sender whatsappconnector.Sender, search whatsappconnector.Searcher) *WhatsAppMCPTools {
	return &WhatsAppMCPTools{store: store, sender: sender, search: search}
}

func (t *WhatsAppMCPTools) AddTo(server *mcp.Server) {
	if t.search != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        whatsappconnector.ToolSearch,
			Title:       "Search WhatsApp chats",
			Description: whatsappconnector.ToolSearchDescription,
		}, t.SearchWhatsApp)
	}
	if t.sender != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        whatsappconnector.ToolSendMessage,
			Title:       "Send WhatsApp message",
			Description: whatsappconnector.ToolSendMessageDescription,
		}, t.SendWhatsAppMessage)
	}
}

func (t *WhatsAppMCPTools) RemoveFrom(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(whatsappconnector.ToolSearch, whatsappconnector.ToolSendMessage)
	}
}

func (t *WhatsAppMCPTools) SearchWhatsApp(ctx context.Context, _ *mcp.CallToolRequest, input WhatsAppSearchInput) (*mcp.CallToolResult, WhatsAppSearchOutput, error) {
	query := strings.TrimSpace(input.Query)
	selected, err := selectMCPConnection(ctx, t.store, whatsappconnector.ProviderID, whatsappconnector.ProviderName, input.Account)
	if err != nil {
		return nil, WhatsAppSearchOutput{}, err
	}
	out := WhatsAppSearchOutput{Provider: whatsappconnector.ProviderID, Accounts: selected.Connections, Query: query}
	applyWhatsAppSearchAccount(&out, selected)
	if !selected.Connected || selected.AccountRequired {
		return textResult(selected.Text), out, nil
	}
	if t.search == nil {
		return textResult("WhatsApp search is not enabled in this runtime."), out, nil
	}
	out.SearcherAvailable = true
	result, err := t.search.Search(ctx, whatsappconnector.SearchRequest{
		Connection: selected.Connection,
		Query:      query,
		Limit:      whatsappconnector.SearchLimit(input.Limit),
	})
	if err != nil {
		return nil, WhatsAppSearchOutput{}, err
	}
	out.Results = result.Items
	if len(out.Results) == 0 {
		return textResult(fmt.Sprintf("No WhatsApp chats or contacts matched %q.", query)), out, nil
	}
	return textResult(fmt.Sprintf("Found %d WhatsApp result(s). Use a result phone or jid with %s recipient.", len(out.Results), whatsappconnector.ToolSendMessage)), out, nil
}

func (t *WhatsAppMCPTools) SendWhatsAppMessage(ctx context.Context, _ *mcp.CallToolRequest, input WhatsAppSendMessageInput) (*mcp.CallToolResult, WhatsAppSendMessageOutput, error) {
	recipient := strings.TrimSpace(input.Recipient)
	if recipient == "" {
		return nil, WhatsAppSendMessageOutput{}, errors.New("recipient is required")
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return nil, WhatsAppSendMessageOutput{}, errors.New("message is required")
	}
	selected, err := selectMCPConnection(ctx, t.store, whatsappconnector.ProviderID, whatsappconnector.ProviderName, input.Account)
	if err != nil {
		return nil, WhatsAppSendMessageOutput{}, err
	}
	out := WhatsAppSendMessageOutput{Provider: whatsappconnector.ProviderID, Accounts: selected.Connections, Recipient: recipient}
	applyWhatsAppSendAccount(&out, selected)
	if !selected.Connected || selected.AccountRequired {
		return textResult(selected.Text), out, nil
	}
	if t.sender == nil {
		return textResult("WhatsApp messaging is not enabled in this runtime."), out, nil
	}
	out.SenderAvailable = true
	result, err := t.sender.SendMessage(ctx, whatsappconnector.SendMessageRequest{
		Connection: selected.Connection,
		Recipient:  recipient,
		Message:    message,
	})
	if err != nil {
		return nil, WhatsAppSendMessageOutput{}, err
	}
	out.Sent = true
	out.MessageID = result.MessageID
	out.JID = result.JID
	out.SentAt = result.SentAt
	return textResult(fmt.Sprintf("Sent WhatsApp message to %s.", recipient)), out, nil
}

func applyWhatsAppSearchAccount(out *WhatsAppSearchOutput, selected mcpAccountSelection) {
	out.Connected = selected.Connected
	out.AccountRequired = selected.AccountRequired
	out.AccountID = selected.Connection.AccountID
	out.Alias = selected.Connection.Alias
}

func applyWhatsAppSendAccount(out *WhatsAppSendMessageOutput, selected mcpAccountSelection) {
	out.Connected = selected.Connected
	out.AccountRequired = selected.AccountRequired
	out.AccountID = selected.Connection.AccountID
	out.Alias = selected.Connection.Alias
}
