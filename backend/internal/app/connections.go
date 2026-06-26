package app

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/connections"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/integrationingest"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

func NewConnectionOAuthService(store *sqlitestore.Store, cfg Config) *connections.OAuthService {
	return connections.NewOAuthService(store, connections.OAuthConfig{
		Gmail: gmailconnector.OAuthClientConfig{
			ClientID:     cfg.Connections.Gmail.OAuthClientID,
			ClientSecret: cfg.Connections.Gmail.OAuthClientSecret,
		},
	})
}

func NewConnectionQRService() *connections.QRService {
	return connections.NewQRService()
}

func NewConnectionConnectService(catalog *connections.Catalog, oauth *connections.OAuthService, qr *connections.QRService) *connections.ConnectService {
	return connections.NewConnectService(catalog, oauth, qr)
}

func NewConnectionService(catalog *connections.Catalog, store *sqlitestore.Store) *connections.Service {
	return connections.NewService(catalog, store)
}

func NewGmailMCPTools(store *sqlitestore.Store) *connections.GmailMCPTools {
	return connections.NewGmailMCPTools(store)
}

func NewChatMCPTools(store *sqlitestore.Store) *connections.ChatMCPTools {
	return connections.NewChatMCPTools(store)
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
