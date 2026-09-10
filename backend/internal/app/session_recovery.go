package app

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/sessionrecovery"
	"github.com/wins/jaz/backend/internal/storage"
	"go.uber.org/fx"
)

func StartSessionRecovery(lc fx.Lifecycle, store storage.SessionStore, manager *acp.Manager, locks *sessionlock.Locks, events *sessionevents.Bus, logger *log.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				if err := sessionrecovery.Resume(ctx, store, manager, locks, events, logger); err != nil && ctx.Err() == nil {
					logger.Error("chat restart recovery failed", "error", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			cancel()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}
