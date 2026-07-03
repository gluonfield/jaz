package app

import (
	"context"
	"strings"
	"testing"

	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
)

func TestProviderMCPToolsRejectDuplicateAdapters(t *testing.T) {
	_, err := NewWhatsAppMCPTools(nil, WhatsAppSenders{Senders: []whatsappconnector.Sender{
		fakeWhatsAppAppSender{},
		fakeWhatsAppAppSender{},
	}}, WhatsAppSearchers{}, WhatsAppReaders{})
	if err == nil || !strings.Contains(err.Error(), "multiple WhatsApp sender providers") {
		t.Fatalf("whatsapp err = %v", err)
	}

	_, err = NewTelegramMCPTools(nil, TelegramSenders{}, TelegramSearchers{Searchers: []telegramconnector.Searcher{
		fakeTelegramAppSearcher{},
		fakeTelegramAppSearcher{},
	}}, TelegramReaders{})
	if err == nil || !strings.Contains(err.Error(), "multiple Telegram searcher providers") {
		t.Fatalf("telegram err = %v", err)
	}
}

type fakeWhatsAppAppSender struct{}

func (fakeWhatsAppAppSender) SendMessage(context.Context, whatsappconnector.SendMessageRequest) (whatsappconnector.SendMessageResult, error) {
	return whatsappconnector.SendMessageResult{}, nil
}

type fakeTelegramAppSearcher struct{}

func (fakeTelegramAppSearcher) Search(context.Context, telegramconnector.SearchRequest) (telegramconnector.SearchResult, error) {
	return telegramconnector.SearchResult{}, nil
}
