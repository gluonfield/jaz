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

// prepareSessionDir resolves server-side projects, permanent homes and utility
// directories, optionally creating a Git worktree.
func (m *Manager) prepareSessionDir(ctx context.Context, req SpawnRequest, cfg AgentConfig, slug, id string) (string, string, error) {
	directory := req.Directory
	branch := strings.TrimSpace(req.Branch)
	if branch != "" && !req.Worktree {
		return "", "", fmt.Errorf("branch requires worktree=true")
	}
	workspace, err := m.resolveCwd("")
	if err != nil {
		return "", "", err
	}
	var abs string
	var projectPath string
	switch {
	case req.Home != "":
		if abs, err = pathsafe.Resolve(req.Home, id); err != nil {
			return "", "", err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", "", err
		}
	case directory != "":
		if filepath.IsAbs(directory) {
			abs = filepath.Clean(directory)
			info, err := os.Stat(abs)
			if err != nil {
				return "", "", err
			}
			if !info.IsDir() {
				return "", "", fmt.Errorf("path is not a directory: %s", abs)
			}
		} else {
			if abs, err = pathsafe.Resolve(workspace, directory); err != nil {
				return "", "", err
			}
			if err := os.MkdirAll(abs, 0o755); err != nil {
				return "", "", err
			}
		}
		if !req.Worktree && directory != "." {
			projectPath = abs
		}
	case cfg.Cwd != "":
		if abs, err = m.resolveCwd(cfg.Cwd); err != nil {
			return "", "", err
		}
	case req.Worktree:
		return "", "", fmt.Errorf("worktree requires a directory pointing at a git repository")
	default:
		if abs, err = pathsafe.Resolve(workspace, slug); err != nil {
			return "", "", err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", "", err
		}
	}
	if !req.Worktree {
		return abs, projectPath, nil
	}
	worktree, repo, err := gitinfo.AddWorktree(ctx, workspace, abs, slug, branch)
	if err != nil {
		return "", "", err
	}
	m.log.Info("created worktree", "dir", abs, "worktree", worktree, "branch", "jaz/"+slug)
	return worktree, repo, nil
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

func (m *Manager) resolveBotWorkspaces(workspaces []string) ([]string, error) {
	resolved := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		var path string
		var err error
		if filepath.IsAbs(workspace) {
			path = filepath.Clean(workspace)
		} else {
			path, err = m.resolveCwd(workspace)
		}
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("bot workspace %q: %w", workspace, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("bot workspace %q is not a directory", workspace)
		}
		resolved = append(resolved, path)
	}
	return resolved, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
