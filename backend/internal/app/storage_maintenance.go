package app

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

const (
	storageMaintenanceYield = 250 * time.Millisecond
	storageMaintenanceRetry = time.Second
	storageMaintenanceIdle  = time.Second
)

func StartStorageMaintenance(lc fx.Lifecycle, store *sqlitestore.Store, logger *log.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				runStorageMaintenance(ctx, store, logger.WithPrefix("storage-maintenance"))
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

type sessionEventMaintenanceStore interface {
	CompactNextSessionEvents(context.Context) (sqlitestore.SessionEventCompaction, error)
	HasPendingSessionEventCompaction(context.Context) (bool, error)
	SessionEventCompactionWake() <-chan struct{}
}

func runStorageMaintenance(ctx context.Context, store sessionEventMaintenanceStore, logger *log.Logger) {
	compacted := 0
	removedTotal := 0
	for ctx.Err() == nil {
		result, err := store.CompactNextSessionEvents(ctx)
		delay := storageMaintenanceYield
		switch {
		case err != nil:
			if ctx.Err() == nil {
				logger.Warn("session event compaction failed", "session", result.ThreadID, "error", err)
			}
			delay = storageMaintenanceRetry
		case result.ThreadID == "":
			if compacted > 0 {
				logger.Info("compacted session events", "sessions", compacted, "removed", removedTotal)
				compacted = 0
				removedTotal = 0
			}
			pending, pendingErr := store.HasPendingSessionEventCompaction(ctx)
			if pendingErr != nil {
				if ctx.Err() == nil {
					logger.Warn("session event compaction state failed", "error", pendingErr)
				}
				delay = storageMaintenanceRetry
			} else if !pending {
				select {
				case <-ctx.Done():
					return
				case <-store.SessionEventCompactionWake():
					continue
				}
			} else {
				delay = storageMaintenanceIdle
			}
		case result.Complete:
			compacted++
			removedTotal += result.Removed
		default:
			removedTotal += result.Removed
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
