package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCheckoutEvidenceKeepsCommitsAndDirtyStateSeparate(t *testing.T) {
	ctx := t.Context()
	workspace := t.TempDir()
	main := filepath.Join(workspace, "repo")
	remote := filepath.Join(workspace, "remote.git")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(workspace, "init", "-q", "-b", "integration", main)
	run(main, "config", "user.email", "t@t")
	run(main, "config", "user.name", "t")
	run(main, "commit", "--allow-empty", "-qm", "base")
	run(workspace, "init", "--bare", "-q", remote)
	run(main, "remote", "add", "origin", remote)
	worktree, _, err := AddWorktree(ctx, workspace, main, "feature", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "feature.txt"), []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(worktree, "add", ".")
	run(worktree, "commit", "-qm", "feature")
	info := Inspect(ctx, worktree)
	if !info.IsWorktree || info.Dirty == nil || *info.Dirty || info.Ahead == nil || *info.Ahead != 1 || info.Head == "" {
		t.Fatalf("committed worktree evidence = %+v", info)
	}
	head := info.Head
	run(worktree, "push", "-qu", "origin", "HEAD")
	info = Inspect(ctx, worktree)
	if info.Upstream != "origin/jaz/feature" || info.PushAhead == nil || *info.PushAhead != 0 || info.Ahead == nil || *info.Ahead != 1 {
		t.Fatalf("pushed but not locally included = %+v", info)
	}
	if err := os.WriteFile(filepath.Join(worktree, "feature.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info = Inspect(ctx, worktree)
	if info.Dirty == nil || !*info.Dirty || info.Ahead == nil || *info.Ahead != 1 || info.Head != head {
		t.Fatalf("dirty state changed commit evidence = %+v", info)
	}
	if info := Inspect(ctx, main); info.IsWorktree || info.Ahead != nil || info.Dirty == nil || *info.Dirty {
		t.Fatalf("clean local checkout must not imply task inclusion = %+v", info)
	}
	if _, err := MergeIntoMain(ctx, worktree, "include dirty work"); err != nil {
		t.Fatal(err)
	}
	info = Inspect(ctx, worktree)
	if info.Ahead == nil || *info.Ahead != 0 || info.Dirty == nil || *info.Dirty || info.PushAhead == nil || *info.PushAhead != 1 {
		t.Fatalf("local inclusion is separate from push state = %+v", info)
	}
	index, err := git(ctx, worktree, "rev-parse", "--git-path", "index")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte("invalid index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info := Inspect(ctx, worktree); !info.Git || info.Dirty != nil || info.Ahead == nil || *info.Ahead != 0 {
		t.Fatalf("failed status must remain unknown = %+v", info)
	}
}

func TestDiffBaseReportsScopeAndRejectsBrokenComparison(t *testing.T) {
	ctx := t.Context()
	workspace := t.TempDir()
	main := filepath.Join(workspace, "repo")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(workspace, "init", "-q", "-b", "integration", main)
	run(main, "commit", "--allow-empty", "-qm", "base")
	base := CurrentRevision(ctx, main).Head
	worktree, _, err := AddWorktree(ctx, workspace, main, "feature", "")
	if err != nil {
		t.Fatal(err)
	}
	summary, err := DiffSummary(ctx, worktree)
	if err != nil || summary.Base != base || summary.BaseKind != "fork" {
		t.Fatalf("worktree diff base = %+v, %v", summary, err)
	}
	summary, err = DiffSummary(ctx, main)
	if err != nil || summary.Base != base || summary.BaseKind != "head" || len(summary.Files) != 0 {
		t.Fatalf("local checkout HEAD diff = %+v, %v", summary, err)
	}
	run(main, "switch", "--orphan", "unrelated")
	run(main, "commit", "--allow-empty", "-qm", "unrelated")
	if summary, err := DiffSummary(ctx, worktree); err == nil {
		t.Fatalf("broken merge base became a successful HEAD diff: %+v", summary)
	}
}
