package worktrees

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/pathsafe"
	"github.com/wins/jaz/backend/internal/storage"
)

type Worktree struct {
	Cwd         string
	ProjectPath string
	Branch      string
}

type sessionReader interface {
	ListSessions(storage.SessionFilter) ([]storage.Session, error)
}

func Managed(workspace string, session storage.Session) (Worktree, bool) {
	if session.RuntimeRef == nil || session.RuntimeRef.Cwd == "" || workspace == "" || session.Slug == "" {
		return Worktree{}, false
	}
	root, err := filepath.Abs(filepath.Join(workspace, ".worktrees"))
	if err != nil {
		return Worktree{}, false
	}
	cwd, err := filepath.Abs(session.RuntimeRef.Cwd)
	if err != nil || !pathsafe.Within(root, cwd) {
		return Worktree{}, false
	}
	relative, err := filepath.Rel(root, cwd)
	if err != nil || relative == "." {
		return Worktree{}, false
	}
	directory, _, _ := strings.Cut(relative, string(filepath.Separator))
	return Worktree{
		Cwd:         filepath.Join(root, directory),
		ProjectPath: strings.TrimSpace(session.RuntimeRef.ProjectPath),
		Branch:      "jaz/" + session.Slug,
	}, true
}

// AddWorktree names the directory after its owning thread. A reviewer can
// share that directory while keeping its own slug and project path.
func Owner(store sessionReader, workspace string, worktree Worktree) (Worktree, error) {
	slug := filepath.Base(worktree.Cwd)
	if worktree.Branch == "jaz/"+slug {
		return worktree, nil
	}
	for _, archived := range []bool{false, true} {
		sessions, err := store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true, Archived: archived})
		if err != nil {
			return Worktree{}, err
		}
		for _, session := range sessions {
			owner, ok := Managed(workspace, session)
			if ok && session.Slug == slug && owner.Cwd == worktree.Cwd {
				return owner, nil
			}
		}
	}
	return Worktree{}, fmt.Errorf("no owning thread for worktree %s", worktree.Cwd)
}

func Ensure(ctx context.Context, store sessionReader, workspace string, session storage.Session) error {
	worktree, ok := Managed(workspace, session)
	if !ok {
		return nil
	}
	if _, err := os.Stat(worktree.Cwd); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	worktree, err := Owner(store, workspace, worktree)
	if err != nil {
		return err
	}
	if worktree.ProjectPath == "" {
		return fmt.Errorf("session worktree is missing and has no project path")
	}
	return gitinfo.RestoreManagedWorktree(ctx, worktree.ProjectPath, worktree.Cwd, worktree.Branch)
}
