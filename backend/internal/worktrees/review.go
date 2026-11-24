package worktrees

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/wins/jaz/backend/internal/storage"
)

func Review(workspace string, sessions []storage.Session) ([]string, error) {
	root, err := filepath.Abs(filepath.Join(workspace, ".worktrees"))
	if err != nil {
		return nil, err
	}
	owners := make(map[string]bool)
	for _, session := range sessions {
		worktree, ok := Managed(workspace, session)
		if ok && worktree.Branch == "jaz/"+filepath.Base(worktree.Cwd) {
			owners[worktree.Cwd] = true
		}
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		cwd := filepath.Join(root, entry.Name())
		if entry.IsDir() && !owners[cwd] {
			if marker, err := os.Lstat(filepath.Join(cwd, ".git")); err == nil && marker.Mode().IsRegular() {
				paths = append(paths, cwd)
			}
		}
	}
	jobs, err := os.ReadDir(jobRoot(workspace))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, job := range jobs {
		home := filepath.Join(jobRoot(workspace), job.Name())
		checkout := filepath.Join(home, ".worktrees", job.Name())
		if job.IsDir() {
			if info, err := os.Lstat(checkout); err == nil && info.IsDir() {
				paths = append(paths, checkout)
			}
		}
	}
	slices.Sort(paths)
	return paths, nil
}
