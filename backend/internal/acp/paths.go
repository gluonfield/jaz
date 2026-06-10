package acp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/pathsafe"
)

// prepareSessionDir resolves where a spawned session works. An explicit
// directory (relative to the workspace, confined to it) is created if
// missing; without one each session gets a fresh directory named after its
// slug. worktree=true swaps the directory for a disposable git worktree on a
// session branch.
func (m *Manager) prepareSessionDir(req SpawnRequest, cfg AgentConfig, slug string) (string, error) {
	directory := strings.TrimSpace(req.Directory)
	workspace, err := m.resolveCwd("")
	if err != nil {
		return "", err
	}
	var abs string
	switch {
	case directory != "":
		if abs, err = pathsafe.Resolve(workspace, directory); err != nil {
			return "", err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", err
		}
	case cfg.Cwd != "":
		if abs, err = m.resolveCwd(cfg.Cwd); err != nil {
			return "", err
		}
	case req.Worktree:
		return "", fmt.Errorf("worktree requires a directory pointing at a git repository")
	default:
		if abs, err = pathsafe.Resolve(workspace, slug); err != nil {
			return "", err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", err
		}
	}
	if !req.Worktree {
		return abs, nil
	}
	worktree, err := gitinfo.AddWorktree(context.Background(), workspace, abs, slug)
	if err != nil {
		return "", err
	}
	m.log.Info("created worktree", "dir", abs, "worktree", worktree, "branch", "jaz/"+slug)
	return worktree, nil
}

func (m *Manager) resolveCwd(configured string) (string, error) {
	cwd := firstNonEmpty(configured, m.cfg.Workspace)
	if cwd == "" {
		cwd = "."
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	if m.cfg.Workspace == "" {
		return abs, nil
	}
	workspace, err := filepath.Abs(m.cfg.Workspace)
	if err != nil {
		return "", err
	}
	if !pathsafe.Within(workspace, abs) {
		return "", fmt.Errorf("acp cwd escapes workspace: %s", cwd)
	}
	return abs, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
