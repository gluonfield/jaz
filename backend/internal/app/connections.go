package app

import (
	"github.com/wins/jaz/backend/internal/connections"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
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

func NewConnectionService(catalog *connections.Catalog, store *sqlitestore.Store) *connections.Service {
	return connections.NewService(catalog, store)
}

func NewGmailMCPTools(store *sqlitestore.Store) *connections.GmailMCPTools {
	return connections.NewGmailMCPTools(store)
}
