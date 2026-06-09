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
