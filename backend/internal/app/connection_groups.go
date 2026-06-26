package app

import (
	"github.com/wins/jaz/backend/internal/connections"
	"go.uber.org/fx"
)

type ConnectionQRProviders struct {
	fx.In

	Providers []connections.QRProvider `group:"connection_qr_providers"`
}

type ChatSenders struct {
	fx.In

	Senders []connections.ChatSender `group:"chat_senders"`
}

type ConnectionSessionDisconnecters struct {
	fx.In

	Disconnecters []connections.SessionDisconnecter `group:"connection_session_disconnecters"`
}

type ChatProviderOut struct {
	fx.Out

	QR            []connections.QRProvider          `group:"connection_qr_providers,flatten"`
	Senders       []connections.ChatSender          `group:"chat_senders,flatten"`
	Disconnecters []connections.SessionDisconnecter `group:"connection_session_disconnecters,flatten"`
}
