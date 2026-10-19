package worktrees

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
)

type Runtime interface {
	SessionUsed(string) bool
	WithStoppedSessions([]string, func() error) error
}

type Pruner struct {
	Store     storage.SessionStore
	Workspace string
	Runtime   Runtime
	Lock      func(...string) func()
	Log       *log.Logger
}

type directory struct {
	worktree  Worktree
	sessions  []storage.Session
	attention time.Time
}

func (p Pruner) Prune(ctx context.Context) error {
	groups := map[string]*directory{}
	for _, archived := range []bool{false, true} {
		sessions, err := p.Store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true, Archived: archived})
		if err != nil {
			return err
		}
		for _, session := range sessions {
			worktree, ok := Managed(p.Workspace, session)
			if !ok {
				continue
			}
			item := groups[worktree.Cwd]
			if item == nil {
				item = &directory{worktree: worktree}
				groups[worktree.Cwd] = item
			}
			item.sessions = append(item.sessions, session)
			if at := storage.SessionAttentionAt(session); at.After(item.attention) {
				item.attention = at
			}
		}
	}
	var capped []*directory
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
		allArchived := true
		eligible := true
		for _, session := range item.sessions {
			allArchived = allArchived && session.Archived
			eligible = eligible && p.eligible(session)
		}
		if !eligible {
			continue
		}
		if allArchived {
			p.remove(ctx, item)
		} else {
			capped = append(capped, item)
		}
	}
	sort.Slice(capped, func(i, j int) bool { return capped[i].attention.After(capped[j].attention) })
	for _, item := range capped[min(15, len(capped)):] {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.remove(ctx, item)
	}
	return nil
}

func (p Pruner) eligible(session storage.Session) bool {
	if session.Pinned || session.Status == storage.StatusRunning || session.Turn != nil || len(session.QueuedMessages) != 0 || session.PendingSteer != nil {
		return false
	}
	switch session.SourceType {
	case storage.SourceBotWorker:
		return session.Archived
	case "":
		return p.Runtime == nil || !p.Runtime.SessionUsed(session.ID)
	default:
		return false
	}
}

func (p Pruner) remove(ctx context.Context, item *directory) {
	ids := make([]string, 0, len(item.sessions))
	for _, session := range item.sessions {
		ids = append(ids, session.ID)
	}
	defer p.Lock(ids...)()
	remove := func() error {
		for _, previous := range item.sessions {
			session, err := p.Store.LoadSession(previous.ID)
			if err != nil {
				return err
			}
			current, ok := Managed(p.Workspace, session)
			if !ok || current.Cwd != item.worktree.Cwd || !p.eligible(session) || session.Archived != previous.Archived || !session.LastAttentionAt.Equal(previous.LastAttentionAt) {
				return nil
			}
		}
		branch := gitinfo.Inspect(ctx, item.worktree.Cwd).Branch
		if branch != "jaz/"+filepath.Base(item.worktree.Cwd) {
			return nil
		}
		for _, session := range item.sessions {
			if branch != "jaz/"+session.Slug {
				continue
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
