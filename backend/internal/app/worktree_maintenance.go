package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/worktrees"
)

type botWorkArchiver interface {
	ArchiveFinishedBotWork(context.Context, time.Time, string) error
	ListSessions(storage.SessionFilter) ([]storage.Session, error)
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
		}
		if err := worktrees.SweepJobs(sweep, workspace, store, logger); err != nil {
			logger.Warn("worktree job cleanup failed", "error", err)
		}
		prune(sweep)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
