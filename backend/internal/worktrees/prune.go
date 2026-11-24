package worktrees

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
)

type Runtime interface {
	WithStoppedSessions([]string, func() error) error
}

type Pruner struct {
	Store     storage.SessionStore
	Workspace string
	Runtime   Runtime
	Lock      func(...string) func()
	Terminal  func(string) bool
	Log       *log.Logger
}

type directory struct {
	worktree Worktree
	sessions []storage.Session
}

func (p Pruner) Prune(ctx context.Context) error {
	groups, err := managedDirectories(p.Store, p.Workspace)
	if err != nil {
		return err
	}
	for _, item := range groups {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := os.Stat(item.worktree.Cwd); err != nil {
			if !os.IsNotExist(err) {
				p.Log.Warn("stat worktree failed", "path", item.worktree.Cwd, "error", err)
			}
			continue
		}
		if !slices.ContainsFunc(item.sessions, retained) {
			p.remove(ctx, item)
		}
	}
	return nil
}

func managedDirectories(store sessionReader, workspace string) (map[string]*directory, error) {
	groups := map[string]*directory{}
	for _, archived := range []bool{false, true} {
		sessions, err := store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true, Archived: archived})
		if err != nil {
			return nil, err
		}
		for _, session := range sessions {
			worktree, ok := Managed(workspace, session)
			if !ok {
				continue
			}
			item := groups[worktree.Cwd]
			if item == nil {
				item = &directory{worktree: worktree}
				groups[worktree.Cwd] = item
			}
			item.sessions = append(item.sessions, session)
		}
	}
	return groups, nil
}

func retained(session storage.Session) bool {
	return !session.Archived || session.Pinned || session.Status == storage.StatusRunning ||
		session.Turn != nil || len(session.QueuedMessages) != 0 ||
		session.PendingSteer != nil || (session.SourceType != "" && session.SourceType != storage.SourceBotWorker)
}

func (p Pruner) remove(ctx context.Context, item *directory) {
	ids := make([]string, 0, len(item.sessions))
	for _, session := range item.sessions {
		ids = append(ids, session.ID)
	}
	defer p.Lock(ids...)()
	remove := func() error {
		defer LockCheckout(p.Workspace, item.worktree.Cwd)()
		currentGroups, err := managedDirectories(p.Store, p.Workspace)
		if err != nil {
			return err
		}
		currentItem := currentGroups[item.worktree.Cwd]
		if currentItem == nil || len(currentItem.sessions) != len(ids) {
			return nil
		}
		for _, previous := range item.sessions {
			session, err := p.Store.LoadSession(previous.ID)
			if err != nil {
				return err
			}
			current, ok := Managed(p.Workspace, session)
			if !ok || current.Cwd != item.worktree.Cwd || retained(session) || p.Terminal(session.ID) {
				return nil
			}
		}
		branch := "jaz/" + filepath.Base(item.worktree.Cwd)
		for _, session := range item.sessions {
			if branch != "jaz/"+session.Slug {
				continue
			}
			owner, _ := Managed(p.Workspace, session)
			if err := gitinfo.CheckWorktree(ctx, owner.Cwd, owner.ProjectPath); err != nil {
				return err
			}
			message := strings.TrimSpace(session.Title)
			if message == "" {
				message = "jaz session changes"
			}
			if err := gitinfo.RemoveManagedWorktree(ctx, item.worktree.Cwd, branch, message); err != nil {
				return err
			}
			p.Log.Info("removed worktree", "session", session.ID, "path", item.worktree.Cwd)
			return nil
		}
		return nil
	}
	var err error
	if p.Runtime == nil {
		err = remove()
	} else {
		err = p.Runtime.WithStoppedSessions(ids, remove)
	}
	if err != nil {
		p.Log.Warn("remove worktree failed", "path", item.worktree.Cwd, "error", err)
	}
}
