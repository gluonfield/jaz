package pathsafe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfinesToRoot(t *testing.T) {
	root := t.TempDir()

	abs, err := Resolve(root, "sub/dir")
	if err != nil {
		t.Fatalf("Resolve in-root: %v", err)
	}
	if want := filepath.Join(root, "sub", "dir"); abs != want {
		t.Fatalf("Resolve = %q, want %q", abs, want)
	}

	if got, err := Resolve(root, ""); err != nil || got != root {
		t.Fatalf("Resolve root = %q, %v; want %q, nil", got, err, root)
	}

	for _, escape := range []string{"../outside", "sub/../../outside", filepath.Join(filepath.Dir(root), "elsewhere")} {
		if _, err := Resolve(root, escape); err == nil {
			t.Fatalf("Resolve(%q) should have been rejected", escape)
		}
	}
}

func TestSubdirsSkipsFilesAndDotfiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"beta", "alpha", ".git"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// alpha is a git repo root; beta is a plain directory.
	if err := os.Mkdir(filepath.Join(root, "alpha", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := Subdirs(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Dir{{Name: "alpha", Git: true}, {Name: "beta", Git: false}}
	if len(dirs) != len(want) || dirs[0] != want[0] || dirs[1] != want[1] {
		t.Fatalf("Subdirs = %v, want %v (sorted, no files, no dotfiles, alpha flagged git)", dirs, want)
	}

	if _, err := Subdirs(filepath.Join(root, "missing")); err == nil {
		t.Fatal("Subdirs on a missing directory should error")
	}
}

func TestTreeSkipsAndCapsDepth(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"a/b/c", "node_modules/pkg", ".git", "src"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"a/b/c/d.txt", "node_modules/pkg/x.js", ".git/config", "src/main.go"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	entries, truncated, err := Tree(root, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("walk should not be truncated")
	}
	got := make(map[string]bool, len(entries))
	for _, entry := range entries {
		got[entry.Path] = entry.Dir
	}
	for path, dir := range map[string]bool{"a": true, "a/b": true, "a/b/c": true, "src": true, "src/main.go": false} {
		if isDir, ok := got[path]; !ok || isDir != dir {
			t.Fatalf("Tree missing %q (dir=%v); got %v", path, dir, got)
		}
	}
	// node_modules and dot-dirs are skipped entirely; depth 3 lists a/b/c but
	// not its children.
	for _, path := range []string{"node_modules", "node_modules/pkg/x.js", ".git", ".git/config", "a/b/c/d.txt"} {
		if _, ok := got[path]; ok {
			t.Fatalf("Tree should not list %q; got %v", path, got)
		}
	}
}

func TestTreeTruncatesBreadthFirst(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := os.MkdirAll(filepath.Join(root, name, "deep"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	entries, truncated, err := Tree(root, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("walk should be truncated")
	}
	// BFS contract: the cap keeps every shallow entry over any deep one.
	want := []Entry{{Path: "alpha", Dir: true}, {Path: "beta", Dir: true}, {Path: "gamma", Dir: true}}
	if len(entries) != len(want) || entries[0] != want[0] || entries[1] != want[1] || entries[2] != want[2] {
		t.Fatalf("Tree = %v, want shallow entries %v", entries, want)
	}

	if _, _, err := Tree(filepath.Join(root, "missing"), 3, 10); err == nil {
		t.Fatal("Tree on a missing root should error")
	}
}

func TestEntriesFromFiles(t *testing.T) {
	files := []string{"src/main.go", "src/util/io.go", "README.md"}
	entries, truncated := EntriesFromFiles(files, 100)
	if truncated {
		t.Fatal("index should not be truncated")
	}
	want := []Entry{
		{Path: "README.md", Dir: false},
		{Path: "src", Dir: true},
		{Path: "src/main.go", Dir: false},
		{Path: "src/util", Dir: true},
		{Path: "src/util/io.go", Dir: false},
	}
	if len(entries) != len(want) {
		t.Fatalf("EntriesFromFiles = %v, want %v", entries, want)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("EntriesFromFiles[%d] = %v, want %v (shallow-first, dirs derived)", i, entries[i], want[i])
		}
	}

	// The cap keeps shallow entries: deep files drop before top-level ones.
	capped, truncated := EntriesFromFiles(files, 2)
	if !truncated || len(capped) != 2 || capped[0].Path != "README.md" || capped[1].Path != "src" {
		t.Fatalf("capped index = %v (truncated=%v), want [README.md src] truncated", capped, truncated)
	}
}

func TestIsGitRepo(t *testing.T) {
	root := t.TempDir()
	if IsGitRepo(root) {
		t.Fatal("plain directory should not be a git repo")
	}
	// A .git file (worktree/submodule) counts the same as a .git directory.
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsGitRepo(root) {
		t.Fatal("directory with a .git entry should be a git repo")
	}
}
