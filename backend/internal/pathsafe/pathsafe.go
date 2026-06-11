// Package pathsafe resolves and lists filesystem paths confined to a root
// directory, so callers can accept untrusted relative (or absolute) paths
// without escaping the directory they're scoped to — a jaz workspace, an
// agent's working directory, and so on.
package pathsafe

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
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

// Entry is a file or directory inside a walked tree; Path is slash-separated
// and relative to the walk root.
type Entry struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

// skipNames are directories whole subtrees of which are never worth indexing:
// dependency caches and build output.
var skipNames = map[string]struct{}{
	"node_modules": {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	"target":       {},
	"out":          {},
	"__pycache__":  {},
}

// Tree lists the files and directories under root, at most maxDepth levels
// deep and maxEntries entries total, skipping dotfiles and well-known
// dependency/build directories. The walk is breadth-first so that when the
// entry cap truncates it, shallow entries survive. The bool reports whether
// the cap truncated the walk. Symlinked directories are listed but not
// recursed into. Unreadable subdirectories are skipped; only an unreadable
// root errors.
func Tree(root string, maxDepth, maxEntries int) ([]Entry, bool, error) {
	type frame struct {
		abs   string
		rel   string
		depth int
	}
	out := []Entry{}
	queue := []frame{{abs: root, rel: "", depth: 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(cur.abs)
		if err != nil {
			if cur.rel == "" {
				return nil, false, err
			}
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if _, skip := skipNames[name]; skip && entry.IsDir() {
				continue
			}
			if len(out) == maxEntries {
				return out, true, nil
			}
			rel := path.Join(cur.rel, name)
			out = append(out, Entry{Path: rel, Dir: entry.IsDir()})
			if entry.IsDir() && cur.depth+1 < maxDepth {
				queue = append(queue, frame{abs: filepath.Join(cur.abs, name), rel: rel, depth: cur.depth + 1})
			}
		}
	}
	return out, false, nil
}

// EntriesFromFiles builds a file/directory index from a flat list of
// slash-relative file paths (e.g. `git ls-files` output), deriving every
// ancestor directory. Entries are ordered shallow-first (depth, then path) so
// the maxEntries cap keeps the tree's surface — the same contract as Tree's
// breadth-first walk. The bool reports whether the cap truncated the index.
func EntriesFromFiles(files []string, maxEntries int) ([]Entry, bool) {
	dirs := make(map[string]struct{})
	out := make([]Entry, 0, len(files))
	for _, file := range files {
		file = strings.Trim(path.Clean(file), "/")
		if file == "" || file == "." {
			continue
		}
		out = append(out, Entry{Path: file, Dir: false})
		for dir := path.Dir(file); dir != "." && dir != "/"; dir = path.Dir(dir) {
			dirs[dir] = struct{}{}
		}
	}
	for dir := range dirs {
		out = append(out, Entry{Path: dir, Dir: true})
	}
	sort.Slice(out, func(i, j int) bool {
		di, dj := strings.Count(out[i].Path, "/"), strings.Count(out[j].Path, "/")
		if di != dj {
			return di < dj
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > maxEntries {
		return out[:maxEntries], true
	}
	return out, false
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
