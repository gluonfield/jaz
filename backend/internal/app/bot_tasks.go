package app

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/bots"
	"go.uber.org/fx"
)

func StartBotTaskWatch(lc fx.Lifecycle, service *bots.Service, recovered <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				select {
				case <-recovered:
					service.RunWatch(ctx, 30*time.Second)
				case <-ctx.Done():
				}
			}()
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
