package app

import (
	"github.com/wins/jaz/backend/internal/connections"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/integrationingest"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func NewConnectionOAuthService(store *sqlitestore.Store, cfg Config) *connections.OAuthService {
	return connections.NewOAuthService(store, connections.OAuthConfig{
		Gmail: gmailconnector.OAuthClientConfig{
			ClientID:     cfg.Connections.Gmail.OAuthClientID,
			ClientSecret: cfg.Connections.Gmail.OAuthClientSecret,
		},
	})
}

func NewConnectionQRService(providers ConnectionQRProviders) *connections.QRService {
	return connections.NewQRService(providers.Providers...)
}

func NewConnectionConnectService(catalog *connections.Catalog, oauth *connections.OAuthService, qr *connections.QRService) *connections.ConnectService {
	return connections.NewConnectService(catalog, oauth, qr)
}

func NewConnectionService(catalog *connections.Catalog, store *sqlitestore.Store, disconnecters ConnectionSessionDisconnecters) *connections.Service {
	return connections.NewService(catalog, store, disconnecters.Disconnecters...)
}

func NewGmailMCPTools(store *sqlitestore.Store, raw integrationingest.RawWriter) *connections.GmailMCPTools {
	return connections.NewGmailMCPTools(store, raw)
}

func NewChatMCPTools(store *sqlitestore.Store, senders ChatSenders) *connections.ChatMCPTools {
	return connections.NewChatMCPTools(store, senders.Senders...)
}
