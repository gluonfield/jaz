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
		Scopes:    []string{"send"},
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
		Scopes:    []string{"send"},
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
		Scopes:    []string{"contacts"},
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

func TestWhatsAppMCPToolsReadsRecentThroughAdapter(t *testing.T) {
	reader := &fakeWhatsAppReader{}
	tools := NewWhatsAppMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "whatsapp:personal",
		Provider:  whatsappconnector.ProviderID,
		AccountID: "15550101111",
		Alias:     "personal",
		Scopes:    []string{"contacts", "messages", "send"},
	}}}, nil, nil, reader)
	result, out, err := tools.ReadWhatsAppRecent(context.Background(), nil, WhatsAppReadRecentInput{
		Chat:  "15550102222@s.whatsapp.net",
		Limit: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || !out.ReaderAvailable || out.Chat != "15550102222@s.whatsapp.net" || len(out.Messages) != 1 {
		t.Fatalf("output = %#v", out)
	}
	if reader.got.Connection.ID != "whatsapp:personal" || reader.got.Chat != "15550102222@s.whatsapp.net" || reader.got.Limit != 200 {
		t.Fatalf("request = %#v", reader.got)
	}
	if got := toolText(result); !strings.Contains(got, "Read 1 recent WhatsApp message") {
		t.Fatalf("text = %q", got)
	}
}

func TestWhatsAppMCPToolsDeniesReadWhenMessageScopeMissing(t *testing.T) {
	reader := &fakeWhatsAppReader{}
	tools := NewWhatsAppMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "whatsapp:personal",
		Provider:  whatsappconnector.ProviderID,
		AccountID: "15550101111",
		Alias:     "personal",
		Scopes:    []string{"contacts", "send"},
	}}}, nil, nil, reader)
	result, out, err := tools.ReadWhatsAppRecent(context.Background(), nil, WhatsAppReadRecentInput{Chat: "15550102222@s.whatsapp.net"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ReaderAvailable || len(out.Messages) != 0 || reader.got.Chat != "" {
		t.Fatalf("output = %#v request = %#v", out, reader.got)
	}
	if got := toolText(result); !strings.Contains(got, "message read access is disabled") {
		t.Fatalf("text = %q", got)
	}
}

func TestTelegramMCPToolsReportsMissingRuntimeSearcher(t *testing.T) {
	tools := NewTelegramMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
		Scopes:    []string{"contacts"},
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

func TestTelegramMCPToolsSearchesThroughAdapter(t *testing.T) {
	searcher := &fakeTelegramSearcher{}
	tools := NewTelegramMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
		Scopes:    []string{"contacts"},
	}}}, nil, searcher)
	result, out, err := tools.SearchTelegram(context.Background(), nil, TelegramSearchInput{
		Query: " alice ",
		Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || !out.SearcherAvailable || out.AccountID != "12345" || out.Query != "alice" || len(out.Results) != 1 {
		t.Fatalf("output = %#v", out)
	}
	if searcher.got.Connection.ID != "telegram:personal" || searcher.got.Query != "alice" || searcher.got.Limit != 25 {
		t.Fatalf("request = %#v", searcher.got)
	}
	if out.Results[0].Recipient != "@alice" {
		t.Fatalf("results = %#v", out.Results)
	}
	if got := toolText(result); !strings.Contains(got, "Found 1 Telegram result") {
		t.Fatalf("text = %q", got)
	}
}

func TestTelegramMCPToolsDeniesSearchWhenContactsScopeMissing(t *testing.T) {
	searcher := &fakeTelegramSearcher{}
	tools := NewTelegramMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
		Scopes:    []string{"messages", "send"},
	}}}, nil, searcher)
	result, out, err := tools.SearchTelegram(context.Background(), nil, TelegramSearchInput{Query: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if out.SearcherAvailable || len(out.Results) != 0 || searcher.got.Query != "" {
		t.Fatalf("output = %#v request = %#v", out, searcher.got)
	}
	if got := toolText(result); !strings.Contains(got, "contact/search access is disabled") {
		t.Fatalf("text = %q", got)
	}
}

func TestTelegramMCPToolsReadsRecentThroughAdapter(t *testing.T) {
	reader := &fakeTelegramReader{}
	tools := NewTelegramMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
		Scopes:    []string{"contacts", "messages", "send"},
	}}}, nil, nil, reader)
	result, out, err := tools.ReadTelegramRecent(context.Background(), nil, TelegramReadRecentInput{
		Peer:  "user:42:99",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Connected || !out.ReaderAvailable || out.PeerID != "user:42" || len(out.Messages) != 1 {
		t.Fatalf("output = %#v", out)
	}
	if reader.got.Connection.ID != "telegram:personal" || reader.got.Peer != "user:42:99" || reader.got.Limit != 5 {
		t.Fatalf("request = %#v", reader.got)
	}
	if got := toolText(result); !strings.Contains(got, "Read 1 recent Telegram message") {
		t.Fatalf("text = %q", got)
	}
}

func TestTelegramMCPToolsDeniesReadWhenMessageScopeMissing(t *testing.T) {
	reader := &fakeTelegramReader{}
	tools := NewTelegramMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
		Scopes:    []string{"contacts", "send"},
	}}}, nil, nil, reader)
	result, out, err := tools.ReadTelegramRecent(context.Background(), nil, TelegramReadRecentInput{Peer: "user:42:99"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ReaderAvailable || len(out.Messages) != 0 || reader.got.Peer != "" {
		t.Fatalf("output = %#v request = %#v", out, reader.got)
	}
	if got := toolText(result); !strings.Contains(got, "message read access is disabled") {
		t.Fatalf("text = %q", got)
	}
}

func TestTelegramMCPToolsDeniesSendWhenSendScopeMissing(t *testing.T) {
	sender := &fakeTelegramSender{}
	tools := NewTelegramMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
		Scopes:    []string{"contacts", "messages"},
	}}}, sender, nil)
	result, out, err := tools.SendTelegramMessage(context.Background(), nil, TelegramSendMessageInput{
		Recipient: "@alice",
		Message:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.SenderAvailable || out.Sent || sender.got.Recipient != "" {
		t.Fatalf("output = %#v request = %#v", out, sender.got)
	}
	if got := toolText(result); !strings.Contains(got, "message send access is disabled") {
		t.Fatalf("text = %q", got)
	}
}

func TestTelegramMCPToolsFailsClosedWhenScopesEmpty(t *testing.T) {
	searcher := &fakeTelegramSearcher{}
	reader := &fakeTelegramReader{}
	sender := &fakeTelegramSender{}
	store := &testConnectionStore{connections: []integrations.Connection{{
		ID:        "telegram:personal",
		Provider:  telegramconnector.ProviderID,
		AccountID: "12345",
		Alias:     "personal",
	}}}
	tools := NewTelegramMCPTools(store, sender, searcher, reader)

	result, searchOut, err := tools.SearchTelegram(context.Background(), nil, TelegramSearchInput{Query: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if searchOut.SearcherAvailable || len(searchOut.Results) != 0 || searcher.got.Query != "" {
		t.Fatalf("search output = %#v request = %#v", searchOut, searcher.got)
	}
	if got := toolText(result); !strings.Contains(got, "contact/search access is disabled") {
		t.Fatalf("search text = %q", got)
	}

	result, readOut, err := tools.ReadTelegramRecent(context.Background(), nil, TelegramReadRecentInput{Peer: "user:42:99"})
	if err != nil {
		t.Fatal(err)
	}
	if readOut.ReaderAvailable || len(readOut.Messages) != 0 || reader.got.Peer != "" {
		t.Fatalf("read output = %#v request = %#v", readOut, reader.got)
	}
	if got := toolText(result); !strings.Contains(got, "message read access is disabled") {
		t.Fatalf("read text = %q", got)
	}

	result, sendOut, err := tools.SendTelegramMessage(context.Background(), nil, TelegramSendMessageInput{
		Recipient: "@alice",
		Message:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sendOut.SenderAvailable || sendOut.Sent || sender.got.Recipient != "" {
		t.Fatalf("send output = %#v request = %#v", sendOut, sender.got)
	}
	if got := toolText(result); !strings.Contains(got, "message send access is disabled") {
		t.Fatalf("send text = %q", got)
	}
}

type fakeTelegramSender struct {
	got telegramconnector.SendMessageRequest
}

type fakeTelegramSearcher struct {
	got telegramconnector.SearchRequest
}

type fakeTelegramReader struct {
	got telegramconnector.ReadRecentRequest
}

func (s *fakeTelegramSearcher) Search(_ context.Context, req telegramconnector.SearchRequest) (telegramconnector.SearchResult, error) {
	s.got = req
	return telegramconnector.SearchResult{Items: []telegramconnector.SearchItem{{
		Kind:      telegramconnector.SearchItemPerson,
		Name:      "Alice",
		Username:  "alice",
		Recipient: "@alice",
		PeerID:    "user:42",
	}}}, nil
}

func (r *fakeTelegramReader) ReadRecent(_ context.Context, req telegramconnector.ReadRecentRequest) (telegramconnector.ReadRecentResult, error) {
	r.got = req
	return telegramconnector.ReadRecentResult{
		PeerID: "user:42",
		Messages: []telegramconnector.ReadRecentMessage{{
			MessageID: "1",
			SentAt:    time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC),
			Text:      "hello",
		}},
	}, nil
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

type fakeWhatsAppReader struct {
	got whatsappconnector.ReadRecentRequest
}

func (r *fakeWhatsAppReader) ReadRecent(_ context.Context, req whatsappconnector.ReadRecentRequest) (whatsappconnector.ReadRecentResult, error) {
	r.got = req
	return whatsappconnector.ReadRecentResult{
		Chat: "15550102222@s.whatsapp.net",
		Messages: []whatsappconnector.ReadRecentMessage{{
			MessageID: "1",
			SentAt:    time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC),
			Text:      "hello",
		}},
	}, nil
}
