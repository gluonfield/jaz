package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/charmbracelet/log"
)

type botWorkArchiver interface {
	ArchiveFinishedBotWork(context.Context, time.Time, string) error
}

func RunWorktreeMaintenance(ctx context.Context, store botWorkArchiver, workspace string, prune func(context.Context), logger *log.Logger) {
	root, err := filepath.Abs(filepath.Join(workspace, ".worktrees"))
	if err != nil {
		logger.Warn("resolve worktree root failed", "error", err)
		return
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for ctx.Err() == nil {
		sweep, cancel := context.WithTimeout(ctx, 2*time.Minute)
		if err := store.ArchiveFinishedBotWork(sweep, time.Now().Add(-7*24*time.Hour), root); err != nil {
			logger.Warn("archive finished bot work failed", "error", err)
		} else {
			prune(sweep)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
