package app

import (
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/connections"
	googleconnector "github.com/wins/jaz/backend/internal/connectors/google"
	slackconnector "github.com/wins/jaz/backend/internal/connectors/slack"
	"github.com/wins/jaz/backend/internal/integrationingest"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func NewConnectionOAuthService(store *sqlitestore.Store, cfg Config) *connections.OAuthService {
	brokerURL := strings.TrimSpace(cfg.Connections.OAuthRedirectBrokerURL)
	if brokerURL == "" {
		brokerURL = connections.DefaultOAuthRedirectBroker
	}
	return connections.NewOAuthService(store, connections.OAuthConfig{
		Calendar: googleconnector.OAuthClientConfig{
			ClientID:     cfg.Connections.Calendar.OAuthClientID,
			ClientSecret: cfg.Connections.Calendar.OAuthClientSecret,
		},
		Gmail: googleconnector.OAuthClientConfig{
			ClientID:     cfg.Connections.Gmail.OAuthClientID,
			ClientSecret: cfg.Connections.Gmail.OAuthClientSecret,
		},
		Slack: slackconnector.OAuthClientConfig{
			ClientID: cfg.Connections.Slack.OAuthClientID,
		},
		RedirectBrokerURL: brokerURL,
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

func NewCalendarMCPTools(store *sqlitestore.Store) *connections.CalendarMCPTools {
	return connections.NewCalendarMCPTools(store)
}

func NewWhatsAppMCPTools(
	store *sqlitestore.Store,
	whatsAppSenders WhatsAppSenders,
	whatsAppSearchers WhatsAppSearchers,
) (*connections.WhatsAppMCPTools, error) {
	whatsAppSender, err := singleProvider("WhatsApp", "sender", whatsAppSenders.Senders)
	if err != nil {
		return nil, err
	}
	whatsAppSearch, err := singleProvider("WhatsApp", "searcher", whatsAppSearchers.Searchers)
	if err != nil {
		return nil, err
	}
	return connections.NewWhatsAppMCPTools(store, whatsAppSender, whatsAppSearch), nil
}

func NewTelegramMCPTools(
	store *sqlitestore.Store,
	telegramSenders TelegramSenders,
	telegramSearchers TelegramSearchers,
) (*connections.TelegramMCPTools, error) {
	telegramSender, err := singleProvider("Telegram", "sender", telegramSenders.Senders)
	if err != nil {
		return nil, err
	}
	telegramSearch, err := singleProvider("Telegram", "searcher", telegramSearchers.Searchers)
	if err != nil {
		return nil, err
	}
	return connections.NewTelegramMCPTools(store, telegramSender, telegramSearch), nil
}

func singleProvider[T any](provider, role string, items []T) (T, error) {
	var zero T
	switch len(items) {
	case 0:
		return zero, nil
	case 1:
		return items[0], nil
	default:
		return zero, fmt.Errorf("multiple %s %s providers registered", provider, role)
	}
}
