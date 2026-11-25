package worktrees

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/filelock"
	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/subprocess"
)

const (
	pendingJob  = "pending.json"
	finishedJob = "finished.json"
	releasedJob = "released.json"
)

func jobRoot(workspace string) string {
	return filepath.Join(workspace, ".worktrees", ".jaz-jobs")
}

func IsJobDirectory(workspace, cwd string) bool {
	return filepath.Base(checkoutPath(workspace, cwd)) == filepath.Base(jobRoot(workspace))
}

func newJobHome(workspace string) (string, error) {
	root, err := filepath.Abs(jobRoot(workspace))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(root, "job-")
}

func writeRegistration(home string, job storage.WorktreeRegistration) error {
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(home, pendingJob), raw, 0o600)
}

func finishJob(home string) error {
	pending := filepath.Join(home, pendingJob)
	now := time.Now()
	if err := os.Chtimes(pending, now, now); err != nil {
		return err
	}
	return os.Rename(pending, filepath.Join(home, finishedJob))
}

func RunJob(ctx context.Context, workspace, repo, base string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("worktree job requires a command")
	}
	home, err := newJobHome(workspace)
	if err != nil {
		return err
	}
	cache := filepath.Join(home, "cache")
	if err := os.Mkdir(cache, 0o700); err != nil {
		return err
	}
	slug := filepath.Base(home)
	worktree, _, err := gitinfo.AddRegisteredWorktree(ctx, home, repo, slug, base, func(cwd, project string) error {
		return writeRegistration(home, storage.WorktreeRegistration{Path: cwd, Project: project, Branch: "jaz/" + slug})
	})
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = worktree
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Env = append(cmd.Environ(), "JAZ_WORKTREE_CACHE="+cache)
	process := subprocess.New(cmd)
	runErr := cmd.Start()
	if runErr == nil {
		if err := process.Started(); err != nil {
			stopErr := process.Terminate()
			return errors.Join(err, stopErr, cmd.Wait())
		}
		runErr = cmd.Wait()
		if err := process.Terminate(); err != nil {
			return errors.Join(runErr, err)
		}
	}
	if ctx.Err() != nil {
		return errors.Join(runErr, ctx.Err())
	}
	if err := finishJob(home); err != nil {
		return errors.Join(runErr, err)
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	if _, err := releaseFinishedJob(cleanup, workspace, home, nil); err != nil {
		fmt.Fprintf(stderr, "kept job checkout %s: %v\n", worktree, err)
	}
	return runErr
}

func releaseFinishedJob(ctx context.Context, workspace, home string, store sessionReader) (time.Time, error) {
	released := filepath.Join(home, releasedJob)
	finished := filepath.Join(home, finishedJob)
	info, err := os.Stat(finished)
	if os.IsNotExist(err) {
		info, err = os.Stat(released)
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		if err != nil {
			return time.Time{}, err
		}
		return info.ModTime(), nil
	}
	if err != nil {
		return time.Time{}, err
	}
	unlock, err := filelock.Lock(filepath.Join(home, "cleanup.lock"))
	if err != nil {
		return time.Time{}, err
	}
	defer unlock()
	raw, err := os.ReadFile(finished)
	if os.IsNotExist(err) {
		info, err := os.Stat(released)
		if err != nil {
			return time.Time{}, err
		}
		return info.ModTime(), nil
	}
	if err != nil {
		return time.Time{}, err
	}
	var job storage.WorktreeRegistration
	if err := json.Unmarshal(raw, &job); err != nil {
		return time.Time{}, err
	}
	if err := releaseRegisteredCheckout(ctx, workspace, home, job, store); err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), os.Rename(finished, released)
}

func releaseRegisteredCheckout(ctx context.Context, workspace, home string, job storage.WorktreeRegistration, store sessionReader) error {
	if _, err := os.Lstat(job.Path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if job.MergedHead == "" {
		if job.Path != filepath.Join(home, ".worktrees", filepath.Base(home)) {
			return fmt.Errorf("job checkout path does not match its registration")
		}
	} else {
		if store == nil || checkoutPath(workspace, job.Path) != job.Path || IsJobDirectory(workspace, job.Path) {
			return fmt.Errorf("adopted checkout is outside the workspace worktrees")
		}
		defer LockCheckout(workspace, job.Path)()
		if err := checkAdoption(ctx, workspace, home, job, store); err != nil {
			return err
		}
	}
	if err := gitinfo.CheckWorktree(ctx, job.Path, job.Project); err != nil {
		return err
	}
	return gitinfo.RemoveManagedWorktree(ctx, job.Path, job.Branch, "Save worktree job changes")
}

func SweepJobs(ctx context.Context, workspace string, store sessionReader, logger *log.Logger) error {
	root, err := filepath.Abs(jobRoot(workspace))
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var caches []jobCache
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			continue
		}
		home := filepath.Join(root, entry.Name())
		finished, err := releaseFinishedJob(ctx, workspace, home, store)
		if err != nil {
			logger.Warn("kept job checkout", "path", home, "error", err)
			continue
		}
		if !finished.IsZero() {
			caches = append(caches, jobCache{path: filepath.Join(home, "cache"), finished: finished})
		}
	}
	return trimJobCaches(ctx, root, caches, jobCacheBudget, minimumFree)
}
