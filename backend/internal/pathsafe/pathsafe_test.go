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
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := Subdirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 || dirs[0] != "alpha" || dirs[1] != "beta" {
		t.Fatalf("Subdirs = %v, want sorted [alpha beta] (no files, no dotfiles)", dirs)
	}

	if _, err := Subdirs(filepath.Join(root, "missing")); err == nil {
		t.Fatal("Subdirs on a missing directory should error")
	}
}
