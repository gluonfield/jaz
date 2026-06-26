package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/wins/jaz/backend/internal/connections"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func (p *Provider) SendMessage(ctx context.Context, req connections.ChatSendRequest) (connections.ChatSendResult, error) {
	client, err := p.clientForConnection(ctx, req.Connection)
	if err != nil {
		return connections.ChatSendResult{}, err
	}
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return connections.ChatSendResult{}, err
		}
	}
	to, err := recipientJID(req.Recipient)
	if err != nil {
		return connections.ChatSendResult{}, err
	}
	resp, err := client.SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(req.Message)})
	if err != nil {
		return connections.ChatSendResult{}, err
	}
	return connections.ChatSendResult{
		MessageID:      string(resp.ID),
		ConversationID: to.String(),
		SentAt:         resp.Timestamp,
	}, nil
}

func recipientJID(value string) (waTypes.JID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return waTypes.JID{}, fmt.Errorf("recipient is required")
	}
	if strings.Contains(value, "@") {
		return waTypes.ParseJID(value)
	}
	number := digits(value)
	if number == "" {
		return waTypes.JID{}, fmt.Errorf("whatsapp recipient must be a phone number or JID")
	}
	return waTypes.NewJID(number, waTypes.DefaultUserServer), nil
}

func digits(value string) string {
	var out strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}
