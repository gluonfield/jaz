package gitinfo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorktreeCreationRequiresDurableRegistration(t *testing.T) {
	repo := commitFixture(t)
	workspace := t.TempDir()
	want := filepath.Join(workspace, ".worktrees", "new")
	refused := errors.New("save failed")
	register := func(cwd, project string) error {
		expected, err := filepath.EvalSymlinks(repo)
		if err != nil {
			t.Fatal(err)
		}
		if cwd != want || project != expected {
			t.Fatalf("registration = %q %q", cwd, project)
		}
		if _, err := os.Lstat(cwd); !os.IsNotExist(err) {
			t.Fatalf("worktree already exists before registration: %v", err)
		}
		return refused
	}
	if _, _, err := AddRegisteredWorktree(t.Context(), workspace, repo, "new", "", register); !errors.Is(err, refused) {
		t.Fatalf("creation error = %v", err)
	}
	if _, err := os.Lstat(want); !os.IsNotExist(err) || BranchExists(t.Context(), repo, "jaz/new") {
		t.Fatalf("failed registration left worktree or branch: %v", err)
	}
}
