// Package pathsafe resolves and lists filesystem paths confined to a root
// directory, so callers can accept untrusted relative (or absolute) paths
// without escaping the directory they're scoped to — a jaz workspace, an
// agent's working directory, and so on.
package pathsafe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve joins name onto root and returns the absolute result, guaranteeing it
// stays within root. An absolute name is cleaned and confined the same way. It
// errors if the result would escape root.
func Resolve(root, name string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(rootAbs, name)
	if filepath.IsAbs(name) {
		abs = filepath.Clean(name)
	}
	if !Within(rootAbs, abs) {
		return "", fmt.Errorf("path escapes the allowed directory: %s", name)
	}
	return abs, nil
}

// Within reports whether path is root itself or a descendant of it.
func Within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Dir is an immediate subdirectory plus whether it is a git repository root.
type Dir struct {
	Name string `json:"name"`
	Git  bool   `json:"git"`
}

// IsGitRepo reports whether dir is a git working tree root: it has a .git entry,
// which may be a directory for a normal repo or a file for a worktree/submodule.
func IsGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Subdirs lists the immediate subdirectories of dir, skipping dotfiles
// (.git, .worktrees, …), flagging each that is a git repository root. Names are
// relative to dir and sorted (os.ReadDir returns entries ordered by filename).
func Subdirs(dir string) ([]Dir, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	dirs := make([]Dir, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dirs = append(dirs, Dir{Name: entry.Name(), Git: IsGitRepo(filepath.Join(dir, entry.Name()))})
	}
	return dirs, nil
}
