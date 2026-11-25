package worktrees

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
)

func Adopt(ctx context.Context, workspace, cwd, project string, sessions []storage.Session) error {
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return err
	}
	project, err = filepath.Abs(project)
	if err != nil {
		return err
	}
	if checkoutPath(workspace, cwd) != cwd || IsJobDirectory(workspace, cwd) {
		return fmt.Errorf("select a standalone checkout directly under the workspace .worktrees")
	}
	for _, session := range sessions {
		if worktree, ok := Managed(workspace, session); ok && worktree.Cwd == cwd {
			return fmt.Errorf("%s belongs to thread %s", cwd, session.ID)
		}
	}
	if err := gitinfo.CheckWorktree(ctx, cwd, project); err != nil {
		return err
	}
	info := gitinfo.Inspect(ctx, cwd)
	if info.Branch == "" || info.Ahead == nil || *info.Ahead != 0 || info.Dirty == nil || *info.Dirty {
		return fmt.Errorf("adoption requires a clean checkout with a branch already merged into the main checkout")
	}
	home, err := newJobHome(workspace)
	if err != nil {
		return err
	}
	if err := writeRegistration(home, storage.WorktreeRegistration{Path: cwd, Project: project, Branch: info.Branch, MergedHead: info.Head}); err != nil {
		return err
	}
	if err := gitinfo.StampWorktreeAdoption(ctx, cwd, filepath.Base(home)); err != nil {
		return err
	}
	return finishJob(home)
}

func checkAdoption(ctx context.Context, workspace, home string, job storage.WorktreeRegistration, store sessionReader) error {
	if err := gitinfo.CheckWorktreeAdoption(ctx, job.Path, filepath.Base(home)); err != nil {
		return err
	}
	groups, err := managedDirectories(store, workspace)
	if err != nil {
		return err
	}
	if groups[job.Path] != nil {
		return fmt.Errorf("adopted checkout has a thread owner")
	}
	info := gitinfo.Inspect(ctx, job.Path)
	if info.Head != job.MergedHead || info.Ahead == nil || *info.Ahead != 0 || info.Dirty == nil || *info.Dirty {
		return fmt.Errorf("adopted checkout changed")
	}
	return nil
}
