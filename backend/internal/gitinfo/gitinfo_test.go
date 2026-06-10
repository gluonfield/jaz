package gitinfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	cases := []struct {
		remote            string
		host, owner, repo string
	}{
		{"git@github.com:wins/jaz.git", "github.com", "wins", "jaz"},
		{"git@github.com:wins/jaz", "github.com", "wins", "jaz"},
		{"https://github.com/wins/jaz.git", "github.com", "wins", "jaz"},
		{"https://github.com/wins/jaz", "github.com", "wins", "jaz"},
		{"ssh://git@github.com/wins/jaz.git", "github.com", "wins", "jaz"},
		{"ssh://git@github.com:22/wins/jaz.git", "github.com", "wins", "jaz"},
		{"https://gitlab.com/group/subgroup/jaz.git", "gitlab.com", "group/subgroup", "jaz"},
		{"git@gitlab.com:group/subgroup/jaz.git", "gitlab.com", "group/subgroup", "jaz"},
		// not forge remotes
		{"/Users/wins/repos/jaz", "", "", ""},
		{"../jaz", "", "", ""},
		{"file:///Users/wins/repos/jaz", "", "", ""},
		{"git@github.com:jaz.git", "", "", ""},
		{"", "", "", ""},
	}
	for _, tc := range cases {
		host, owner, repo := ParseRemote(tc.remote)
		if host != tc.host || owner != tc.owner || repo != tc.repo {
			t.Errorf("ParseRemote(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tc.remote, host, owner, repo, tc.host, tc.owner, tc.repo)
		}
	}
}

func TestInspect(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()

	if info := Inspect(ctx, t.TempDir()); info.Git {
		t.Fatalf("Inspect(non-repo) = %+v, want zero Info", info)
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("remote", "add", "origin", "git@github.com:wins/jaz.git")

	info := Inspect(ctx, dir)
	if !info.Git {
		t.Fatal("Inspect(repo).Git = false, want true")
	}
	if info.Branch != "main" {
		t.Errorf("Branch = %q, want main", info.Branch)
	}
	if info.WebURL != "https://github.com/wins/jaz" {
		t.Errorf("WebURL = %q, want https://github.com/wins/jaz", info.WebURL)
	}
	if info.HasUpstream {
		t.Error("HasUpstream = true for a never-pushed branch")
	}
}

func TestAddWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "repo")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", dir).Run(); err != nil {
		t.Fatal(err)
	}
	run("commit", "--allow-empty", "-m", "init")

	worktree, err := AddWorktree(ctx, workspace, dir, "my-thread")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if want := filepath.Join(workspace, ".worktrees", "my-thread"); worktree != want {
		t.Errorf("worktree path = %q, want %q", worktree, want)
	}
	if info := Inspect(ctx, worktree); info.Branch != "jaz/my-thread" {
		t.Errorf("worktree branch = %q, want jaz/my-thread", info.Branch)
	}

	// A second worktree for the same slug must fail, not silently reuse it.
	if _, err := AddWorktree(ctx, workspace, dir, "my-thread"); err == nil {
		t.Error("AddWorktree with a duplicate slug succeeded, want error")
	}
}

func TestPushAndCommitState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	if err := exec.Command("git", "init", "-q", "--bare", "-b", "main", bare).Run(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "repo")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", dir).Run(); err != nil {
		t.Fatal(err)
	}
	run("commit", "--allow-empty", "-m", "init")
	run("remote", "add", "origin", bare)
	run("push", "-q", "-u", "origin", "main")
	run("checkout", "-q", "-b", "feature")

	// Fresh branch: zero commits over main — nothing to PR, nothing to push.
	if info := Inspect(ctx, dir); !info.NoCommits || info.NeedsPush {
		t.Errorf("fresh branch: no_commits=%v needs_push=%v, want true/false", info.NoCommits, info.NeedsPush)
	}
	run("commit", "--allow-empty", "-m", "work")
	info := Inspect(ctx, dir)
	if info.NoCommits {
		t.Error("NoCommits = true after a commit, want false")
	}
	if info.HasUpstream {
		t.Error("HasUpstream = true before push, want false")
	}
	if !info.NeedsPush {
		t.Error("NeedsPush = false with an unpushed commit, want true")
	}

	if err := Push(ctx, dir); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if info := Inspect(ctx, dir); !info.HasUpstream || info.NeedsPush {
		t.Errorf("after Push: has_upstream=%v needs_push=%v, want true/false", info.HasUpstream, info.NeedsPush)
	}
	// A new commit on a tracked branch re-arms the push.
	run("commit", "--allow-empty", "-m", "more work")
	if info := Inspect(ctx, dir); !info.NeedsPush {
		t.Error("NeedsPush = false when ahead of upstream, want true")
	}
	if _, err := exec.Command("git", "-C", bare, "rev-parse", "--verify", "refs/heads/feature").Output(); err != nil {
		t.Error("feature branch missing on the remote after Push")
	}
}

func TestCommitAllAndMergeIntoMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	// macOS tempdirs are symlinks (/var -> /private/var); resolve so paths
	// compare equal with git's realpath output.
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(workspace, "repo")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", main).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "index.html"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "init")

	worktree, err := AddWorktree(ctx, workspace, main, "wt")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	// Nothing to merge is an error, not a silent success.
	if _, err := MergeIntoMain(ctx, worktree, "noop"); err == nil || !strings.Contains(err.Error(), "nothing to merge") {
		t.Errorf("MergeIntoMain with no changes: err = %v, want nothing-to-merge", err)
	}
	// Merging from the main checkout is rejected — including when the cwd
	// reaches it through a symlink while git reports realpaths.
	if _, err := MergeIntoMain(ctx, main, "x"); err == nil {
		t.Error("MergeIntoMain from the main checkout succeeded, want error")
	}
	link := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(main, link); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeIntoMain(ctx, link, "x"); err == nil {
		t.Error("MergeIntoMain from a symlinked main checkout succeeded, want error")
	}
	if info := Inspect(ctx, link); info.IsWorktree {
		t.Error("Inspect through a symlink flags the main checkout as a worktree")
	}

	// Agent work left dirty (an edit and an untracked file): MergeIntoMain
	// commits it on the worktree branch and fast-forwards main.
	if err := os.WriteFile(filepath.Join(worktree, "index.html"), []byte("orange\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := MergeIntoMain(ctx, worktree, "make it orange")
	if err != nil {
		t.Fatalf("MergeIntoMain: %v", err)
	}
	if root != main {
		t.Errorf("MergeIntoMain root = %q, want %q", root, main)
	}
	if got, _ := os.ReadFile(filepath.Join(main, "index.html")); string(got) != "orange\n" {
		t.Errorf("main index.html = %q, want merged content", got)
	}
	if _, err := os.Stat(filepath.Join(main, "new.txt")); err != nil {
		t.Error("untracked new.txt did not arrive in main")
	}
	if out := git(main, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("main checkout dirty after merge:\n%s", out)
	}
	if log := git(main, "log", "--oneline"); strings.Count(log, "\n") != 2 {
		t.Errorf("main log after fast-forward merge:\n%s", log)
	}
	if log := git(main, "log", "-1", "--format=%s"); strings.TrimSpace(log) != "make it orange" {
		t.Errorf("merge commit subject = %q, want session-title message", strings.TrimSpace(log))
	}
	// Idempotent: a second merge with nothing new is the readable error.
	if _, err := MergeIntoMain(ctx, worktree, "again"); err == nil || !strings.Contains(err.Error(), "nothing to merge") {
		t.Errorf("repeat MergeIntoMain: err = %v, want nothing-to-merge", err)
	}

	// Conflicting work: main diverges on the same line; the merge aborts and
	// the main checkout stays pristine (no MERGE_HEAD, no conflict markers).
	if err := os.WriteFile(filepath.Join(main, "index.html"), []byte("purple\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "main goes purple")
	if err := os.WriteFile(filepath.Join(worktree, "index.html"), []byte("teal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeIntoMain(ctx, worktree, "make it teal"); err == nil {
		t.Error("conflicting MergeIntoMain succeeded, want error")
	}
	if out := git(main, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("main checkout not pristine after aborted merge:\n%s", out)
	}
	if got, _ := os.ReadFile(filepath.Join(main, "index.html")); string(got) != "purple\n" {
		t.Errorf("main index.html = %q after aborted merge, want purple", got)
	}
}

func TestInspectNonOriginRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("remote", "add", "github", "git@github.com:wins/cats.git")

	info := Inspect(context.Background(), dir)
	if info.WebURL != "https://github.com/wins/cats" {
		t.Errorf("WebURL = %q, want https://github.com/wins/cats (first-remote fallback)", info.WebURL)
	}
}
