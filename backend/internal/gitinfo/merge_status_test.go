package gitinfo

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInspectCountsAgainstActualMergeTarget(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	main := filepath.Join(workspace, "repo")
	remote := filepath.Join(workspace, "remote.git")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(workspace, "init", "-q", "-b", "main", main)
	run(main, "commit", "--allow-empty", "-qm", "base")
	run(workspace, "init", "--bare", "-q", "-b", "main", remote)
	run(main, "remote", "add", "origin", remote)
	run(main, "push", "-qu", "origin", "main")
	run(main, "checkout", "-qb", "integration")
	worktree, _, err := AddWorktree(ctx, workspace, main, "feature", "")
	if err != nil {
		t.Fatal(err)
	}
	run(worktree, "commit", "--allow-empty", "-qm", "feature")
	run(main, "branch", "-f", "main", "jaz/feature")
	info := Inspect(ctx, worktree)
	if info.MainBranch != "integration" || !info.NoCommits || info.Ahead == nil || *info.Ahead != 1 {
		t.Fatalf("Inspect = %+v; want feature in default main but still outside integration", info)
	}
	if _, err := MergeIntoMain(ctx, worktree, "merge feature"); err != nil {
		t.Fatal(err)
	}
	if info := Inspect(ctx, worktree); info.Ahead == nil || *info.Ahead != 0 {
		t.Fatalf("Inspect after merge = %+v; want known zero outstanding commits", info)
	}
}
