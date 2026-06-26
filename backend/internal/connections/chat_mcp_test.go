package connections

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestChatMCPToolsReportsNotConnected(t *testing.T) {
	result, out, err := NewChatMCPTools(&gmailMCPStore{}).SendWhatsAppMessage(context.Background(), nil, ChatSendMessageInput{
		Recipient: "+447700900123",
		Message:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Connected || out.SenderAvailable || out.Sent || out.Provider != whatsapp.ProviderID {
		t.Fatalf("output = %#v", out)
	}
	if got := gmailToolText(result); !strings.Contains(got, "WhatsApp is not connected") {
		t.Fatalf("text = %q", got)
	}
}

func TestChatMCPToolsRequiresAccountWhenMultipleAccountsConnected(t *testing.T) {
	tools := NewChatMCPTools(&gmailMCPStore{connections: []integrations.Connection{{
		ID:        "whatsapp:personal",
		Provider:  whatsapp.ProviderID,
		AccountID: "+447700900123",
		Alias:     "personal",
	}, {
		ID:        "whatsapp:work",
		Provider:  whatsapp.ProviderID,
		AccountID: "+447700900456",
		Alias:     "work",
	}}})
	result, out, err := tools.SendWhatsAppMessage(context.Background(), nil, ChatSendMessageInput{
		Recipient: "+447700900789",
		Message:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || !out.AccountRequired || out.SenderAvailable || out.Sent || len(out.Accounts) != 2 {
		t.Fatalf("output = %#v", out)
	}
	if got := gmailToolText(result); !strings.Contains(got, "Specify account") || !strings.Contains(got, "personal") || !strings.Contains(got, "work") {
		t.Fatalf("text = %q", got)
	}
}

func TestChatMCPToolsReportsMissingRuntimeSender(t *testing.T) {
	tools := NewChatMCPTools(&gmailMCPStore{connections: []integrations.Connection{{
		ID:        "whatsapp:personal",
		Provider:  whatsapp.ProviderID,
		AccountID: "+447700900123",
		Alias:     "personal",
	}}})
	result, out, err := tools.SendWhatsAppMessage(context.Background(), nil, ChatSendMessageInput{
		Recipient: "+447700900789",
		Message:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || out.SenderAvailable || out.Sent || out.AccountID != "+447700900123" || out.Alias != "personal" {
		t.Fatalf("output = %#v", out)
	}
	if got := gmailToolText(result); !strings.Contains(got, "messaging is not enabled in this runtime") {
		t.Fatalf("text = %q", got)
	}
}

func TestChatMCPToolsSendsThroughProviderAdapter(t *testing.T) {
	sender := &fakeChatSender{provider: telegram.ProviderID}
	tools := NewChatMCPTools(&gmailMCPStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegram.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
	}}}, sender)
	result, out, err := tools.SendTelegramMessage(context.Background(), nil, ChatSendMessageInput{
		Recipient: " @augustinas ",
		Message:   " hello ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || !out.SenderAvailable || !out.Sent || out.MessageID != "msg_1" || out.ConversationID != "chat_1" {
		t.Fatalf("output = %#v", out)
	}
	if sender.got.Connection.ID != "telegram:personal" || sender.got.Recipient != "@augustinas" || sender.got.Message != "hello" {
		t.Fatalf("request = %#v", sender.got)
	}
	if got := gmailToolText(result); !strings.Contains(got, "Sent Telegram message to @augustinas") {
		t.Fatalf("text = %q", got)
	}
}

type fakeChatSender struct {
	provider string
	got      ChatSendRequest
}

func (s *fakeChatSender) ProviderID() string {
	return s.provider
}

func (s *fakeChatSender) SendMessage(_ context.Context, req ChatSendRequest) (ChatSendResult, error) {
	s.got = req
	return ChatSendResult{
		MessageID:      "msg_1",
		ConversationID: "chat_1",
		SentAt:         time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC),
	}, nil
}
