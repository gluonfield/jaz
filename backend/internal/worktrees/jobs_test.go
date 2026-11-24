package worktrees

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestWorktreeJobCleansUpAfterFailure(t *testing.T) {
	if cache := os.Getenv("JAZ_WORKTREE_CACHE"); cache != "" && os.Getenv("JAZ_TEST_WORKTREE_JOB") == "run" {
		if err := os.WriteFile(filepath.Join(cache, "cache.dat"), []byte("generated"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(".git"); err != nil {
			t.Fatal(err)
		}
		os.Exit(17)
	}
	t.Setenv("JAZ_TEST_WORKTREE_JOB", "run")
	repo, git := jobRepo(t)
	workspace := t.TempDir()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{binary, "-test.run=^TestWorktreeJobCleansUpAfterFailure$"}
	var output bytes.Buffer
	err = RunJob(t.Context(), workspace, repo, "", args, nil, &output, &output)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 17 {
		t.Fatalf("job error = %v, output %q", err, output.String())
	}
	entries, err := os.ReadDir(jobRoot(workspace))
	if err != nil || len(entries) != 1 {
		t.Fatalf("job entries = %v, %v", entries, err)
	}
	name := entries[0].Name()
	home := filepath.Join(jobRoot(workspace), name)
	if _, err := os.Stat(filepath.Join(home, ".worktrees", name)); !os.IsNotExist(err) {
		t.Fatalf("finished job still has a checkout: %v, %s", err, output.String())
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "cache.dat")); err != nil {
		t.Fatalf("cache was not stored in its managed directory: %v", err)
	}
	git("show-ref", "--verify", "refs/heads/jaz/"+name)
}

func TestAdoptedWorktreeIsReleasedOnlyOnce(t *testing.T) {
	repo, git := jobRepo(t)
	workspace := t.TempDir()
	cwd := filepath.Join(workspace, ".worktrees", "legacy")
	git("worktree", "add", "-b", "legacy", cwd)
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := Adopt(t.Context(), workspace, cwd, repo, nil); err != nil {
		t.Fatal(err)
	}
	logger := log.New(&bytes.Buffer{})
	thread, err := store.CreateSession(storage.CreateSession{Slug: "review", RuntimeRef: &storage.RuntimeRef{Cwd: cwd, ProjectPath: repo}})
	if err != nil {
		t.Fatal(err)
	}
	if err := SweepJobs(t.Context(), workspace, store, logger); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cwd); err != nil {
		t.Fatalf("checkout was released after a thread attached: %v", err)
	}
	thread.RuntimeRef.Cwd = repo
	if err := store.SaveSession(thread); err != nil {
		t.Fatal(err)
	}
	if err := SweepJobs(t.Context(), workspace, store, logger); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cwd); !os.IsNotExist(err) {
		t.Fatalf("adopted checkout was retained: %v", err)
	}
	git("worktree", "add", cwd, "legacy")
	if err := SweepJobs(t.Context(), workspace, store, logger); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cwd); err != nil {
		t.Fatalf("a later checkout at the same path was removed: %v", err)
	}
}

func jobRepo(t *testing.T) (string, func(...string)) {
	t.Helper()
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "-q", "-b", "main")
	git("commit", "--allow-empty", "-qm", "base")
	return repo, git
}

func TestWorktreeCacheBudgetEvictsOldestCompletedCache(t *testing.T) {
	root := t.TempDir()
	oldCache, newCache, liveCache := filepath.Join(root, "old"), filepath.Join(root, "new"), filepath.Join(root, "live")
	for _, cache := range []string{oldCache, newCache, liveCache} {
		if err := os.Mkdir(cache, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cache, "generated"), []byte("12345"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	finished := []jobCache{{path: newCache, finished: now}, {path: oldCache, finished: now.Add(-time.Hour)}}
	if err := trimJobCaches(t.Context(), root, finished, 5, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldCache); !os.IsNotExist(err) {
		t.Fatalf("old cache survived trimming: %v", err)
	}
	for _, cache := range []string{newCache, liveCache} {
		if content, err := os.ReadFile(filepath.Join(cache, "generated")); string(content) != "12345" || err != nil {
			t.Fatalf("kept cache %s changed: %q, %v", cache, content, err)
		}
	}
}
