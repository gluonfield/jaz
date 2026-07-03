package app

import (
	"github.com/wins/jaz/backend/internal/connections"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"go.uber.org/fx"
)

type ConnectionQRProviders struct {
	fx.In

	Providers []connections.QRProvider `group:"connection_qr_providers"`
}

type WhatsAppSenders struct {
	fx.In

	Senders []whatsappconnector.Sender `group:"whatsapp_senders"`
}

type WhatsAppSearchers struct {
	fx.In

	Searchers []whatsappconnector.Searcher `group:"whatsapp_searchers"`
}

type WhatsAppReaders struct {
	fx.In

	Readers []whatsappconnector.Reader `group:"whatsapp_readers"`
}

type TelegramSenders struct {
	fx.In

	Senders []telegramconnector.Sender `group:"telegram_senders"`
}

type TelegramSearchers struct {
	fx.In

	Searchers []telegramconnector.Searcher `group:"telegram_searchers"`
}

type TelegramReaders struct {
	fx.In

	Readers []telegramconnector.Reader `group:"telegram_readers"`
}

type ConnectionSessionDisconnecters struct {
	fx.In

	Disconnecters []connections.SessionDisconnecter `group:"connection_session_disconnecters"`
}

type WhatsAppProviderOut struct {
	fx.Out

	QR            []connections.QRProvider          `group:"connection_qr_providers,flatten"`
	Senders       []whatsappconnector.Sender        `group:"whatsapp_senders,flatten"`
	Searchers     []whatsappconnector.Searcher      `group:"whatsapp_searchers,flatten"`
	Readers       []whatsappconnector.Reader        `group:"whatsapp_readers,flatten"`
	Disconnecters []connections.SessionDisconnecter `group:"connection_session_disconnecters,flatten"`
}

type TelegramProviderOut struct {
	fx.Out

	QR            []connections.QRProvider          `group:"connection_qr_providers,flatten"`
	Senders       []telegramconnector.Sender        `group:"telegram_senders,flatten"`
	Searchers     []telegramconnector.Searcher      `group:"telegram_searchers,flatten"`
	Readers       []telegramconnector.Reader        `group:"telegram_readers,flatten"`
	Disconnecters []connections.SessionDisconnecter `group:"connection_session_disconnecters,flatten"`
}
