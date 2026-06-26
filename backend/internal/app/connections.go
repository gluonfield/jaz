package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/chatproviders/telegram"
	"github.com/wins/jaz/backend/internal/chatproviders/whatsapp"
	"github.com/wins/jaz/backend/internal/connections"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
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

func NewConnectionService(catalog *connections.Catalog, store *sqlitestore.Store, qr *connections.QRService, disconnecters ConnectionSessionDisconnecters) *connections.Service {
	return connections.NewService(catalog, store, qr, disconnecters.Disconnecters...)
}

func NewGmailMCPTools(store *sqlitestore.Store) *connections.GmailMCPTools {
	return connections.NewGmailMCPTools(store)
}

func NewChatMCPTools(store *sqlitestore.Store, senders ChatSenders) *connections.ChatMCPTools {
	return connections.NewChatMCPTools(store, senders.Senders...)
}

func NewIntegrationRawWriter() integrationingest.RawWriter {
	return integrationingest.RawWriter{}
}

func NewWhatsAppChatProvider(lc fx.Lifecycle, layout runtimefiles.Layout, store *sqlitestore.Store, raw integrationingest.RawWriter) (ChatProviderOut, error) {
	provider, err := whatsapp.New(context.Background(), filepath.Join(layout.Connections, "whatsapp"), store, raw)
	if err != nil {
		return ChatProviderOut{}, err
	}
	lc.Append(fx.Hook{
		OnStart: provider.Start,
		OnStop: func(context.Context) error {
			return provider.Close()
		},
	})
	return ChatProviderOut{
		QR:            []connections.QRProvider{provider},
		Senders:       []connections.ChatSender{provider},
		Disconnecters: []connections.SessionDisconnecter{provider},
	}, nil
}

func NewTelegramChatProvider(lc fx.Lifecycle, cfg Config, layout runtimefiles.Layout, store *sqlitestore.Store, raw integrationingest.RawWriter) (ChatProviderOut, error) {
	telegramConfig, ok, err := telegramProviderConfig(cfg)
	if err != nil {
		return ChatProviderOut{}, err
	}
	if !ok {
		return ChatProviderOut{}, nil
	}
	provider, err := telegram.New(filepath.Join(layout.Connections, "telegram"), telegramConfig, store, raw)
	if err != nil {
		return ChatProviderOut{}, err
	}
	lc.Append(fx.Hook{
		OnStart: provider.Start,
		OnStop: func(context.Context) error {
			return provider.Close()
		},
	})
	return ChatProviderOut{
		QR:            []connections.QRProvider{provider},
		Senders:       []connections.ChatSender{provider},
		Disconnecters: []connections.SessionDisconnecter{provider},
	}, nil
}

func telegramProviderConfig(cfg Config) (telegram.Config, bool, error) {
	apiID := cfg.Connections.Telegram.APIID
	apiHash := strings.TrimSpace(cfg.Connections.Telegram.APIHash)
	if apiID == 0 && apiHash == "" {
		return telegram.Config{}, false, nil
	}
	if apiID == 0 || apiHash == "" {
		return telegram.Config{}, false, fmt.Errorf("telegram api id and api hash must both be configured")
	}
	return telegram.Config{APIID: apiID, APIHash: apiHash}, true, nil
}

func NewGmailSyncer(store *sqlitestore.Store) integrationingest.GmailSyncer {
	return integrationingest.GmailSyncer{Store: store}
}

func StartGmailSync(lc fx.Lifecycle, syncer integrationingest.GmailSyncer, logger *log.Logger) {
	var cancel context.CancelFunc
	var done chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			done = make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(syncer.PollInterval())
				defer ticker.Stop()
				for {
					if err := syncer.SyncOnce(ctx); err != nil && ctx.Err() == nil {
						logger.WithPrefix("gmail-sync").Warn("gmail sync failed", "error", err)
					}
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if cancel != nil {
				cancel()
			}
			if done == nil {
				return nil
			}
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}
