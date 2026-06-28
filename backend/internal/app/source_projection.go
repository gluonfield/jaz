package app

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"go.uber.org/fx"
)

const sourceProjectionInterval = 30 * time.Second

func StartSourceProjectionWorker(lc fx.Lifecycle, runner integrationingest.SourceProjectionRunner, logger *log.Logger) {
	var cancel context.CancelFunc
	var done chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			done = make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(sourceProjectionInterval)
				defer ticker.Stop()
				for {
					processed, err := runner.RunOnce(ctx)
					if err != nil && ctx.Err() == nil && logger != nil {
						logger.WithPrefix("source-projection").Warn("source projection failed", "error", err)
					} else {
						logSourceQueueStatus(ctx, logger, "source-projection", runner.Queue, processed)
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
