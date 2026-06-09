package acp

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	return m.addWorktree(workspace, abs, slug)
}

func (m *Manager) addWorktree(workspace, dir, slug string) (string, error) {
	repo, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("worktree requires a git repository at %s: %w", dir, err)
	}
	worktree := filepath.Join(workspace, ".worktrees", slug)
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		return "", err
	}
	if _, err := gitOutput(repo, "worktree", "add", "-b", "jaz/"+slug, worktree); err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}
	m.log.Info("created worktree", "repo", repo, "worktree", worktree, "branch", "jaz/"+slug)
	return worktree, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("git %s: %s", args[0], detail)
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
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
