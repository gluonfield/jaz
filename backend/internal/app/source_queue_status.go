package app

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/sourcequeue"
)

func logSourceQueueStatus(ctx context.Context, logger *log.Logger, prefix string, queue *sourcequeue.Queue, processed int) {
	if logger == nil || queue == nil {
		return
	}
	stats, err := queue.Stats(ctx)
	if err != nil {
		if ctx.Err() == nil {
			logger.WithPrefix(prefix).Warn("source queue status failed", "error", err)
		}
		return
	}
	if processed == 0 && stats.Pending == 0 && stats.Processing == 0 {
		return
	}
	logger.WithPrefix(prefix).Info("source queue progress", "processed", processed, "pending", stats.Pending, "processing", stats.Processing)
}
