package app

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/mobilepush"
	"github.com/wins/jaz/backend/internal/sessionevents"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

func NewMobilePush(config Config, store *sqlitestore.Store, manager *acp.Manager) (*mobilepush.Service, error) {
	sender, err := mobilepush.NewApple(config.Push)
	if err != nil {
		return nil, err
	}
	return mobilepush.NewService(store, sender, manager.AwaitsAnswer), nil
}

func StartMobilePush(lc fx.Lifecycle, service *mobilepush.Service, events *sessionevents.Bus, logger *log.Logger) {
	if !service.Enabled() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	var done <-chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			done = service.Start(ctx, events, func(err error) {
				logger.Warn("mobile push delivery failed", "error", err)
			})
			return nil
		},
		OnStop: func(stop context.Context) error {
			cancel()
			select {
			case <-done:
				return nil
			case <-stop.Done():
				return stop.Err()
			}
		},
	})
}
