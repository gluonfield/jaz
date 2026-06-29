package app

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/memorysource"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

const memorySourceInterval = 2 * time.Minute

func NewMemorySourceRunner(store *sqlitestore.Store, manager *acp.Manager, memory *jazmem.Memory, queue MemorySourceQueue, logger *log.Logger) *memorysource.Runner {
	return memorysource.New(memory.Root(), store, queue.Queue, manager, logger)
}

func StartMemorySourceWorker(lc fx.Lifecycle, runner *memorysource.Runner, logger *log.Logger) {
	if runner == nil {
		return
	}
	var cancel context.CancelFunc
	var done chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			done = make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(memorySourceInterval)
				defer ticker.Stop()
				for {
					processed, err := runner.RunUntilIdle(ctx)
					if err != nil && ctx.Err() == nil && logger != nil {
						logger.WithPrefix("memory-source").Warn("source capture failed", "error", err)
					} else {
						logSourceQueueStatus(ctx, logger, "memory-source", runner.Queue, processed)
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
