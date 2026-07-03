package connections

import (
	"context"
	"strings"
	"testing"
	"time"

	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestWhatsAppMCPToolsReportsNotConnected(t *testing.T) {
	result, out, err := NewWhatsAppMCPTools(&testConnectionStore{}, nil, nil).SendWhatsAppMessage(context.Background(), nil, WhatsAppSendMessageInput{
		Recipient: "+447700900123",
		Message:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Connected || out.SenderAvailable || out.Sent || out.Provider != whatsappconnector.ProviderID {
		t.Fatalf("output = %#v", out)
	}
	if got := toolText(result); !strings.Contains(got, "WhatsApp is not connected") {
		t.Fatalf("text = %q", got)
	}
}

func TestWhatsAppMCPToolsRequiresAccountWhenMultipleAccountsConnected(t *testing.T) {
	tools := NewWhatsAppMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "whatsapp:personal",
		Provider:  whatsappconnector.ProviderID,
		AccountID: "+447700900123",
		Alias:     "personal",
	}, {
		ID:        "whatsapp:work",
		Provider:  whatsappconnector.ProviderID,
		AccountID: "+447700900456",
		Alias:     "work",
	}}}, nil, nil)
	result, out, err := tools.SendWhatsAppMessage(context.Background(), nil, WhatsAppSendMessageInput{
		Recipient: "+447700900789",
		Message:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || !out.AccountRequired || out.SenderAvailable || out.Sent || len(out.Accounts) != 2 {
		t.Fatalf("output = %#v", out)
	}
	if got := toolText(result); !strings.Contains(got, "Specify account") || !strings.Contains(got, "personal") || !strings.Contains(got, "work") {
		t.Fatalf("text = %q", got)
	}
}

func TestWhatsAppMCPToolsReportsMissingRuntimeSender(t *testing.T) {
	tools := NewWhatsAppMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "whatsapp:personal",
		Provider:  whatsappconnector.ProviderID,
		AccountID: "+447700900123",
		Alias:     "personal",
	}}}, nil, nil)
	result, out, err := tools.SendWhatsAppMessage(context.Background(), nil, WhatsAppSendMessageInput{
		Recipient: "+447700900789",
		Message:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || out.SenderAvailable || out.Sent || out.AccountID != "+447700900123" || out.Alias != "personal" {
		t.Fatalf("output = %#v", out)
	}
	if got := toolText(result); !strings.Contains(got, "messaging is not enabled in this runtime") {
		t.Fatalf("text = %q", got)
	}
}

func TestTelegramMCPToolsSendsThroughAdapter(t *testing.T) {
	sender := &fakeTelegramSender{}
	tools := NewTelegramMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
	}}}, sender, nil)
	result, out, err := tools.SendTelegramMessage(context.Background(), nil, TelegramSendMessageInput{
		Recipient: " @augustinas ",
		Message:   " hello ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || !out.SenderAvailable || !out.Sent || out.MessageID != "msg_1" || out.PeerID != "user:1" {
		t.Fatalf("output = %#v", out)
	}
	if sender.got.Connection.ID != "telegram:personal" || sender.got.Recipient != "@augustinas" || sender.got.Message != "hello" {
		t.Fatalf("request = %#v", sender.got)
	}
	if got := toolText(result); !strings.Contains(got, "Sent Telegram message to @augustinas") {
		t.Fatalf("text = %q", got)
	}
}

func TestWhatsAppMCPToolsSearchesThroughAdapter(t *testing.T) {
	searcher := &fakeWhatsAppSearcher{}
	tools := NewWhatsAppMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "whatsapp:personal",
		Provider:  whatsappconnector.ProviderID,
		AccountID: "15550101111",
		Alias:     "personal",
	}}}, nil, searcher)
	result, out, err := tools.SearchWhatsApp(context.Background(), nil, WhatsAppSearchInput{
		Query: " alice ",
		Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || !out.SearcherAvailable || out.AccountID != "15550101111" || out.Query != "alice" || len(out.Results) != 1 {
		t.Fatalf("output = %#v", out)
	}
	if searcher.got.Connection.ID != "whatsapp:personal" || searcher.got.Query != "alice" || searcher.got.Limit != 25 {
		t.Fatalf("request = %#v", searcher.got)
	}
	if out.Results[0].JID != "15550102222@s.whatsapp.net" {
		t.Fatalf("results = %#v", out.Results)
	}
	if got := toolText(result); !strings.Contains(got, "Use a result phone or jid with whatsapp_send_message recipient") {
		t.Fatalf("text = %q", got)
	}
}

func TestTelegramMCPToolsReportsMissingRuntimeSearcher(t *testing.T) {
	tools := NewTelegramMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
	}}}, nil, nil)
	result, out, err := tools.SearchTelegram(context.Background(), nil, TelegramSearchInput{Query: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || out.SearcherAvailable || len(out.Results) != 0 {
		t.Fatalf("output = %#v", out)
	}
	if got := toolText(result); !strings.Contains(got, "search is not enabled in this runtime") {
		t.Fatalf("text = %q", got)
	}
}

type fakeTelegramSender struct {
	got telegramconnector.SendMessageRequest
}

func (s *fakeTelegramSender) SendMessage(_ context.Context, req telegramconnector.SendMessageRequest) (telegramconnector.SendMessageResult, error) {
	s.got = req
	return telegramconnector.SendMessageResult{
		MessageID: "msg_1",
		PeerID:    "user:1",
		SentAt:    time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC),
	}, nil
}

type fakeWhatsAppSearcher struct {
	got whatsappconnector.SearchRequest
}

func (s *fakeWhatsAppSearcher) Search(_ context.Context, req whatsappconnector.SearchRequest) (whatsappconnector.SearchResult, error) {
	s.got = req
	return whatsappconnector.SearchResult{Items: []whatsappconnector.SearchItem{{
		Kind:  whatsappconnector.SearchItemPerson,
		Name:  "Alice",
		Phone: "15550102222",
		JID:   "15550102222@s.whatsapp.net",
	}}}, nil
}
