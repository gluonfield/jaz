package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func (p *Provider) SendMessage(ctx context.Context, req whatsappconnector.SendMessageRequest) (whatsappconnector.SendMessageResult, error) {
	client, err := p.clientForConnection(ctx, req.Connection)
	if err != nil {
		return whatsappconnector.SendMessageResult{}, err
	}
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return whatsappconnector.SendMessageResult{}, err
		}
	}
	to, err := recipientJID(req.Recipient)
	if err != nil {
		return whatsappconnector.SendMessageResult{}, err
	}
	resp, err := client.SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(req.Message)})
	if err != nil {
		return whatsappconnector.SendMessageResult{}, err
	}
	return whatsappconnector.SendMessageResult{
		MessageID: string(resp.ID),
		JID:       to.String(),
		SentAt:    resp.Timestamp,
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
