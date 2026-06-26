package app

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/integrationingest"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

func NewGmailSyncer(store *sqlitestore.Store, raw integrationingest.RawWriter) integrationingest.GmailSyncer {
	return integrationingest.GmailSyncer{Store: store, Writer: raw}
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
