package app

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

const (
	storageMaintenanceYield = 10 * time.Millisecond
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
	CompactNextLegacySessionEvents(context.Context) (sqlitestore.SessionEventCompaction, error)
	HasLegacySessionEventThreads(context.Context) (bool, error)
	CompactACPStates(context.Context) (int, int64, error)
}

func runStorageMaintenance(ctx context.Context, store sessionEventMaintenanceStore, logger *log.Logger) {
	compactACPStates(ctx, store, logger)
	compacted := 0
	removedTotal := 0
	for ctx.Err() == nil {
		result, err := store.CompactNextLegacySessionEvents(ctx)
		delay := storageMaintenanceYield
		switch {
		case err != nil:
			if ctx.Err() == nil {
				logger.Warn("session event compaction failed", "session", result.ThreadID, "error", err)
			}
			delay = storageMaintenanceRetry
		case result.ThreadID == "":
			if compacted > 0 {
				logger.Info("compacted legacy session events", "sessions", compacted, "removed", removedTotal)
				compacted = 0
				removedTotal = 0
			}
			pending, pendingErr := store.HasLegacySessionEventThreads(ctx)
			if pendingErr != nil {
				if ctx.Err() == nil {
					logger.Warn("session event compaction state failed", "error", pendingErr)
				}
				delay = storageMaintenanceRetry
			} else if !pending {
				return
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

func compactACPStates(ctx context.Context, store sessionEventMaintenanceStore, logger *log.Logger) {
	states, bytes, err := store.CompactACPStates(ctx)
	if err != nil {
		if ctx.Err() == nil {
			logger.Warn("acp state compaction failed", "error", err)
		}
		return
	}
	if states > 0 {
		logger.Info("compacted redundant acp state", "sessions", states, "removed_bytes", bytes)
	}
}
