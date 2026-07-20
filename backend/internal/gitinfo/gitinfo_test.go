package gitinfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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

	worktree, repo, err := AddWorktree(ctx, workspace, dir, "my-thread", "")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	wantRepo, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if gotRepo != wantRepo {
		t.Errorf("repo root = %q, want %q", repo, dir)
	}
	if want := filepath.Join(workspace, ".worktrees", "my-thread"); worktree != want {
		t.Errorf("worktree path = %q, want %q", worktree, want)
	}
	if info := Inspect(ctx, worktree); info.Branch != "jaz/my-thread" {
		t.Errorf("worktree branch = %q, want jaz/my-thread", info.Branch)
	}

	// A second worktree for the same slug must fail, not silently reuse it.
	if _, _, err := AddWorktree(ctx, workspace, dir, "my-thread", ""); err == nil {
		t.Error("AddWorktree with a duplicate slug succeeded, want error")
	}
}

func TestAddWorktreeBaseRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "repo")
	run := func(target string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", target, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(target, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(target, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", dir).Run(); err != nil {
		t.Fatal(err)
	}
	write(dir, "base.txt", "base\n")
	run(dir, "add", "-A")
	run(dir, "commit", "-q", "-m", "init")
	run(dir, "switch", "-q", "-c", "feature")
	write(dir, "feature.txt", "feature\n")
	run(dir, "add", "-A")
	run(dir, "commit", "-q", "-m", "feature")
	run(dir, "switch", "-q", "main")

	featureWorktree, repo, err := AddWorktree(ctx, workspace, dir, "from-feature", "feature")
	if err != nil {
		t.Fatalf("AddWorktree(feature): %v", err)
	}
	wantRepo, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if gotRepo != wantRepo {
		t.Errorf("repo root = %q, want %q", repo, dir)
	}
	if _, err := os.Stat(filepath.Join(featureWorktree, "feature.txt")); err != nil {
		t.Fatalf("worktree did not start from feature: %v", err)
	}

	parent, _, err := AddWorktree(ctx, workspace, dir, "parent", "feature")
	if err != nil {
		t.Fatalf("AddWorktree(parent): %v", err)
	}
	write(parent, "parent.txt", "parent\n")
	run(parent, "add", "-A")
	run(parent, "commit", "-q", "-m", "parent")

	child, repo, err := AddWorktree(ctx, workspace, parent, "child", "")
	if err != nil {
		t.Fatalf("AddWorktree(child): %v", err)
	}
	gotRepo, err = filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if gotRepo != wantRepo {
		t.Errorf("source worktree project path = %q, want primary repo %q", repo, dir)
	}
	if _, err := os.Stat(filepath.Join(child, "parent.txt")); err != nil {
		t.Fatalf("empty base ref did not use source worktree HEAD: %v", err)
	}

	mainChild, _, err := AddWorktree(ctx, workspace, parent, "from-main", "main")
	if err != nil {
		t.Fatalf("AddWorktree(main): %v", err)
	}
	if _, err := os.Stat(filepath.Join(mainChild, "parent.txt")); !os.IsNotExist(err) {
		t.Fatalf("explicit main base included parent work: %v", err)
	}
}

func TestRemoveAndRestoreManagedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "repo")
	run := func(target string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", target, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", dir).Run(); err != nil {
		t.Fatal(err)
	}
	run(dir, "config", "user.email", "t@t")
	run(dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(dir, "add", "-A")
	run(dir, "commit", "-q", "-m", "init")

	worktree, _, err := AddWorktree(ctx, workspace, dir, "restore-thread", "")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "file.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "ignored.log"), []byte("cache\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	branch := "jaz/restore-thread"
	if err := RemoveManagedWorktree(ctx, worktree, branch, "snapshot"); err != nil {
		t.Fatalf("RemoveManagedWorktree: %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after remove: %v", err)
	}
	if !BranchExists(ctx, dir, branch) {
		t.Fatalf("branch %q missing after remove", branch)
	}
	if err := RestoreManagedWorktree(ctx, dir, worktree, branch); err != nil {
		t.Fatalf("RestoreManagedWorktree: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(worktree, "file.txt")); err != nil || string(got) != "two\n" {
		t.Fatalf("restored file.txt = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(worktree, "new.txt")); err != nil || string(got) != "new\n" {
		t.Fatalf("restored new.txt = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "ignored.log")); !os.IsNotExist(err) {
		t.Fatalf("ignored file restored or stat failed: %v", err)
	}
	if info := Inspect(ctx, worktree); info.Branch != branch || !info.IsWorktree {
		t.Fatalf("restored info = %+v, want branch %q worktree", info, branch)
	}
}

func TestRemoveManagedWorktreeSnapshotsWithoutUserGitConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	configDir := t.TempDir()
	globalConfig := filepath.Join(configDir, "global")
	if err := os.WriteFile(globalConfig, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("HOME", t.TempDir())

	ctx := context.Background()
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "repo")
	run := func(target string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", target}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", dir).Run(); err != nil {
		t.Fatal(err)
	}
	run(dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-q", "-m", "init")

	worktree, _, err := AddWorktree(ctx, workspace, dir, "snapshot-thread", "")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "file.txt"), []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branch := "jaz/snapshot-thread"
	if err := RemoveManagedWorktree(ctx, worktree, branch, "snapshot"); err != nil {
		t.Fatalf("RemoveManagedWorktree: %v", err)
	}
	if err := RestoreManagedWorktree(ctx, dir, worktree, branch); err != nil {
		t.Fatalf("RestoreManagedWorktree: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(worktree, "file.txt")); err != nil || string(got) != "saved\n" {
		t.Fatalf("restored file.txt = %q, %v", got, err)
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
	git(main, "config", "user.email", "t@t")
	git(main, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(main, "index.html"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "init")

	worktree, _, err := AddWorktree(ctx, workspace, main, "wt", "")
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
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
	} else {
		if _, err := MergeIntoMain(ctx, link, "x"); err == nil {
			t.Error("MergeIntoMain from a symlinked main checkout succeeded, want error")
		}
		if info := Inspect(ctx, link); info.IsWorktree {
			t.Error("Inspect through a symlink flags the main checkout as a worktree")
		}
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
	} else if !strings.Contains(err.Error(), "CONFLICT") {
		t.Errorf("conflicting MergeIntoMain error = %q, want conflict detail", err)
	}
	if out := git(main, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("main checkout not pristine after aborted merge:\n%s", out)
	}
	if got, _ := os.ReadFile(filepath.Join(main, "index.html")); string(got) != "purple\n" {
		t.Errorf("main index.html = %q after aborted merge, want purple", got)
	}
}

func TestMergeFromMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	origin := filepath.Join(workspace, "origin.git")
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
	if err := exec.Command("git", "init", "-q", "--bare", "-b", "main", origin).Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", main).Run(); err != nil {
		t.Fatal(err)
	}
	git(main, "config", "user.email", "t@t")
	git(main, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(main, "index.html"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "init")
	git(main, "remote", "add", "origin", origin)
	git(main, "push", "-q", "-u", "origin", "main")

	worktree, _, err := AddWorktree(ctx, workspace, main, "wt", "")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	sibling, _, err := AddWorktree(ctx, workspace, main, "sibling", "")
	if err != nil {
		t.Fatalf("AddWorktree sibling: %v", err)
	}

	// Even with main: nothing to pull in, and Behind reports 0.
	if info := Inspect(ctx, worktree); info.Behind != 0 {
		t.Errorf("Behind = %d before main advances, want 0", info.Behind)
	}
	if err := MergeFromMain(ctx, worktree, "noop"); err == nil || !strings.Contains(err.Error(), "nothing to merge") {
		t.Errorf("MergeFromMain with main even: err = %v, want nothing-to-merge", err)
	}
	// The main checkout itself has no main to pull from — rejected.
	if err := MergeFromMain(ctx, main, "x"); err == nil {
		t.Error("MergeFromMain from the main checkout succeeded, want error")
	}

	// A sibling hands work off to main; the other worktree is now one commit behind.
	if err := os.WriteFile(filepath.Join(sibling, "feature.txt"), []byte("from main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeIntoMain(ctx, sibling, "main adds feature"); err != nil {
		t.Fatalf("MergeIntoMain sibling: %v", err)
	}
	if count := strings.TrimSpace(git(worktree, "rev-list", "--count", "HEAD..refs/remotes/origin/main")); count != "0" {
		t.Fatalf("commits behind origin/main = %s, want 0", count)
	}
	if info := Inspect(ctx, worktree); info.Behind != 1 {
		t.Errorf("Behind = %d after main advances, want 1", info.Behind)
	}

	// The worktree has its own dirty edit: MergeFromMain commits it on the
	// worktree branch, then pulls main in. Both end up present in the worktree.
	if err := os.WriteFile(filepath.Join(worktree, "notes.txt"), []byte("agent work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MergeFromMain(ctx, worktree, "agent work"); err != nil {
		t.Fatalf("MergeFromMain: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(worktree, "feature.txt")); string(got) != "from main\n" {
		t.Errorf("worktree feature.txt = %q, want main's content", got)
	}
	if got, _ := os.ReadFile(filepath.Join(worktree, "notes.txt")); string(got) != "agent work\n" {
		t.Errorf("worktree notes.txt = %q, want the agent's committed work", got)
	}
	if out := git(worktree, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("worktree dirty after merge:\n%s", out)
	}
	if info := Inspect(ctx, worktree); info.Behind != 0 {
		t.Errorf("Behind = %d after update, want 0", info.Behind)
	}
	// An inbound merge never touches the main checkout.
	if _, err := os.Stat(filepath.Join(main, "notes.txt")); err == nil {
		t.Error("worktree's notes.txt leaked into the main checkout")
	}

	// Conflict: main and the worktree change the same line; the merge aborts
	// and the worktree stays pristine with its own (committed) content.
	if err := os.WriteFile(filepath.Join(main, "index.html"), []byte("purple\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "main goes purple")
	if err := os.WriteFile(filepath.Join(worktree, "index.html"), []byte("teal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MergeFromMain(ctx, worktree, "make it teal"); err == nil {
		t.Error("conflicting MergeFromMain succeeded, want error")
	} else if !strings.Contains(err.Error(), "CONFLICT") {
		t.Errorf("conflicting MergeFromMain error = %q, want conflict detail", err)
	}
	if out := git(worktree, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("worktree not pristine after aborted merge:\n%s", out)
	}
	if got, _ := os.ReadFile(filepath.Join(worktree, "index.html")); string(got) != "teal\n" {
		t.Errorf("worktree index.html = %q after aborted merge, want teal", got)
	}
}

func TestMergeFromMainUsesLocalMainWhenOriginDiverged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(workspace, "origin.git")
	main := filepath.Join(workspace, "repo")
	other := filepath.Join(workspace, "other")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s %v: %v\n%s", dir, args, err, out)
		}
		return string(out)
	}
	write := func(dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := exec.Command("git", "init", "-q", "--bare", "-b", "main", bare).Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", main).Run(); err != nil {
		t.Fatal(err)
	}
	write(main, "settings.txt", "base\n")
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "init")
	git(main, "remote", "add", "origin", bare)
	git(main, "push", "-q", "-u", "origin", "main")

	worktree, _, err := AddWorktree(ctx, workspace, main, "wt", "")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	write(main, "settings.txt", "local main\n")
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "local main")

	if err := exec.Command("git", "clone", "-q", bare, other).Run(); err != nil {
		t.Fatal(err)
	}
	write(other, "settings.txt", "origin main\n")
	git(other, "add", "-A")
	git(other, "commit", "-q", "-m", "origin main")
	git(other, "push", "-q", "origin", "main")
	git(main, "fetch", "-q", "origin", "main")

	if got := git(worktree, "show", "refs/remotes/origin/main:settings.txt"); got != "origin main\n" {
		t.Fatalf("origin/main settings.txt = %q, want origin main", got)
	}
	if info := Inspect(ctx, worktree); info.Behind != 1 {
		t.Fatalf("Inspect = %+v before local main merge, want behind=1", info)
	}
	if err := MergeFromMain(ctx, worktree, "noop"); err != nil {
		t.Fatalf("MergeFromMain = %v, want merge from local main", err)
	}
	if got, _ := os.ReadFile(filepath.Join(worktree, "settings.txt")); string(got) != "local main\n" {
		t.Fatalf("worktree settings.txt = %q, want local main", got)
	}
	if info := Inspect(ctx, worktree); info.Behind != 0 {
		t.Fatalf("Inspect = %+v after local main merge, want behind=0", info)
	}
}

func TestDiffSummaryAndFileDiff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()

	if _, err := DiffSummary(ctx, t.TempDir()); err == nil {
		t.Fatal("DiffSummary(non-repo) should error so the handler can degrade to empty")
	}

	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(workspace, "repo")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", main).Run(); err != nil {
		t.Fatal(err)
	}
	write := func(dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(main, "keep.txt", "one\ntwo\nthree\n")
	write(main, "gone.txt", "bye\n")
	write(main, "moved.txt", "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n")
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "init")

	worktree, _, err := AddWorktree(ctx, workspace, main, "diff-wt", "")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	// Session work in every flavor: a committed edit, a dirty edit on top, a
	// deletion, a rename, an untracked text file, and an untracked binary.
	write(worktree, "keep.txt", "one\nTWO\nthree\n")
	git(worktree, "commit", "-q", "-am", "edit keep")
	write(worktree, "keep.txt", "one\nTWO\nthree\nfour\n")
	git(worktree, "rm", "-q", "gone.txt")
	git(worktree, "mv", "moved.txt", "renamed.txt")
	write(worktree, "fresh.txt", "hi\nthere")
	if err := os.WriteFile(filepath.Join(worktree, "blob.bin"), []byte("a\x00b"), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := DiffSummary(ctx, worktree)
	if err != nil {
		t.Fatalf("DiffSummary: %v", err)
	}
	byPath := map[string]FileChange{}
	for _, file := range summary.Files {
		byPath[file.Path] = file
	}
	// Committed and uncommitted edits both count against the fork point.
	if got := byPath["keep.txt"]; got.Status != "modified" || got.Added != 2 || got.Deleted != 1 {
		t.Errorf("keep.txt = %+v, want modified +2 -1", got)
	}
	if got := byPath["gone.txt"]; got.Status != "deleted" || got.Deleted != 1 {
		t.Errorf("gone.txt = %+v, want deleted -1", got)
	}
	if got := byPath["renamed.txt"]; got.Status != "renamed" || got.OldPath != "moved.txt" {
		t.Errorf("renamed.txt = %+v, want renamed from moved.txt", got)
	}
	if got := byPath["fresh.txt"]; got.Status != "untracked" || got.Added != 2 {
		t.Errorf("fresh.txt = %+v, want untracked +2 (no trailing newline)", got)
	}
	if got := byPath["blob.bin"]; got.Status != "untracked" || !got.Binary || got.Added != 0 {
		t.Errorf("blob.bin = %+v, want untracked binary", got)
	}
	if summary.TotalAdded != 4 || summary.TotalDeleted != 2 {
		t.Errorf("totals = +%d -%d, want +4 -2", summary.TotalAdded, summary.TotalDeleted)
	}

	// Main moving on must not leak into the session's summary (merge-base).
	write(main, "mainonly.txt", "main work\n")
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "main moves")
	after, err := DiffSummary(ctx, worktree)
	if err != nil {
		t.Fatalf("DiffSummary after main moved: %v", err)
	}
	if !reflect.DeepEqual(after, summary) {
		t.Errorf("summary changed when main moved:\n got %+v\nwant %+v", after, summary)
	}

	// The main checkout itself shows only uncommitted work, not history.
	write(main, "mainonly.txt", "main work\nedited\n")
	mainSummary, err := DiffSummary(ctx, main)
	if err != nil {
		t.Fatalf("DiffSummary(main): %v", err)
	}
	if len(mainSummary.Files) != 1 || mainSummary.Files[0].Path != "mainonly.txt" {
		t.Errorf("main summary = %+v, want just the dirty mainonly.txt", mainSummary.Files)
	}

	// Per-file patches: a tracked edit pinned to the summary's base, an
	// untracked file, a rename, and the binary sniff on both diff flavors.
	patch, err := FileDiff(ctx, worktree, FileDiffSpec{Path: "keep.txt", Base: summary.Base})
	if err != nil {
		t.Fatalf("FileDiff(keep.txt): %v", err)
	}
	if !strings.Contains(patch.Patch, "@@") || !strings.Contains(patch.Patch, "+TWO") || !strings.Contains(patch.Patch, "+four") {
		t.Errorf("keep.txt patch missing combined edits:\n%s", patch.Patch)
	}
	patch, err = FileDiff(ctx, worktree, FileDiffSpec{Path: "fresh.txt", Untracked: true})
	if err != nil {
		t.Fatalf("FileDiff(fresh.txt): %v", err)
	}
	if !strings.Contains(patch.Patch, "+hi") {
		t.Errorf("fresh.txt patch should show the untracked file as added:\n%s", patch.Patch)
	}
	patch, err = FileDiff(ctx, worktree, FileDiffSpec{Path: "renamed.txt", OldPath: "moved.txt"})
	if err != nil {
		t.Fatalf("FileDiff(renamed.txt): %v", err)
	}
	if !strings.Contains(patch.Patch, "rename") {
		t.Errorf("renamed.txt patch should mention the rename:\n%s", patch.Patch)
	}
	patch, err = FileDiff(ctx, worktree, FileDiffSpec{Path: "blob.bin", Untracked: true})
	if err != nil {
		t.Fatalf("FileDiff(blob.bin untracked): %v", err)
	}
	if !patch.Binary || patch.Patch != "" {
		t.Errorf("untracked blob.bin = %+v, want binary with empty patch", patch)
	}
	git(worktree, "add", "blob.bin")
	git(worktree, "commit", "-q", "-m", "binary")
	patch, err = FileDiff(ctx, worktree, FileDiffSpec{Path: "blob.bin"})
	if err != nil {
		t.Fatalf("FileDiff(blob.bin): %v", err)
	}
	if !patch.Binary || patch.Patch != "" {
		t.Errorf("blob.bin = %+v, want binary with empty patch", patch)
	}

	// One path, two rows: gone.txt is deleted vs base AND recreated
	// untracked. Each row's patch must tell its own story — the spec's
	// Untracked flag is what keeps them apart.
	write(worktree, "gone.txt", "reborn\n")
	reborn, err := DiffSummary(ctx, worktree)
	if err != nil {
		t.Fatalf("DiffSummary after recreating gone.txt: %v", err)
	}
	rows := map[string]bool{}
	for _, file := range reborn.Files {
		if file.Path == "gone.txt" {
			rows[file.Status] = true
		}
	}
	if len(rows) != 2 || !rows["deleted"] || !rows["untracked"] {
		t.Errorf("gone.txt rows = %v, want deleted + untracked", rows)
	}
	patch, err = FileDiff(ctx, worktree, FileDiffSpec{Path: "gone.txt", Base: reborn.Base})
	if err != nil {
		t.Fatalf("FileDiff(gone.txt deleted): %v", err)
	}
	if !strings.Contains(patch.Patch, "-bye") || strings.Contains(patch.Patch, "+reborn") {
		t.Errorf("deleted row should diff the tracked deletion:\n%s", patch.Patch)
	}
	patch, err = FileDiff(ctx, worktree, FileDiffSpec{Path: "gone.txt", Untracked: true})
	if err != nil {
		t.Fatalf("FileDiff(gone.txt untracked): %v", err)
	}
	if !strings.Contains(patch.Patch, "+reborn") || strings.Contains(patch.Patch, "-bye") {
		t.Errorf("untracked row should diff the recreation:\n%s", patch.Patch)
	}
}

func TestFileDiffKeepsTrailingBlankContext(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x\n\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("y\n\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patch, err := FileDiff(ctx, dir, FileDiffSpec{Path: "f.txt"})
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	// git renders unchanged blank lines as a single space; a whitespace-
	// trimming runner would eat them off the end and the hunk would render
	// shorter than its @@ header claims.
	if !strings.HasSuffix(patch.Patch, "\n \n ") {
		t.Errorf("trailing blank context lines were trimmed:\n%q", patch.Patch)
	}
}

func TestListFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()

	if _, err := ListFiles(ctx, t.TempDir()); err == nil {
		t.Fatal("ListFiles(non-repo) should error so callers fall back to a plain walk")
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
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		".gitignore":            "node_modules/\n",
		"src/main.go":           "package main\n",
		"untracked.txt":         "new\n",
		"node_modules/pkg/x.js": "ignored\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".gitignore", "src/main.go")

	files, err := ListFiles(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(files))
	for _, file := range files {
		got[file] = true
	}
	// Tracked and untracked files are listed; gitignored ones are not.
	for _, want := range []string{".gitignore", "src/main.go", "untracked.txt"} {
		if !got[want] {
			t.Fatalf("ListFiles missing %q; got %v", want, files)
		}
	}
	if got["node_modules/pkg/x.js"] {
		t.Fatalf("ListFiles should respect .gitignore; got %v", files)
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
