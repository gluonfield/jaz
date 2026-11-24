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
	return os.WriteFile(filepath.Join(home, "registration.json"), raw, 0o600)
}

func readRegistration(home string) (storage.WorktreeRegistration, error) {
	raw, err := os.ReadFile(filepath.Join(home, "registration.json"))
	if err != nil {
		return storage.WorktreeRegistration{}, err
	}
	var job storage.WorktreeRegistration
	err = json.Unmarshal(raw, &job)
	return job, err
}

func RunJob(ctx context.Context, workspace, repo, base string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("worktree job requires a command")
	}
	home, err := newJobHome(workspace)
	if err != nil {
		return err
	}
	slug := filepath.Base(home)
	worktree, _, err := gitinfo.AddRegisteredWorktree(ctx, home, repo, slug, base, func(cwd, project string) error {
		return writeRegistration(home, storage.WorktreeRegistration{Path: cwd, Project: project, Branch: "jaz/" + slug})
	})
	if err != nil {
		return err
	}
	cache := filepath.Join(home, "cache")
	if err := os.Mkdir(cache, 0o700); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = worktree
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Env = append(cmd.Environ(), "JAZ_WORKTREE_CACHE="+cache)
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return errors.Join(runErr, ctx.Err())
	}
	if err := os.WriteFile(filepath.Join(home, "finished"), nil, 0o600); err != nil {
		return errors.Join(runErr, err)
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	job, err := readRegistration(home)
	if err == nil {
		err = removeFinishedJobCheckout(cleanup, workspace, home, job)
	}
	if err != nil {
		fmt.Fprintf(stderr, "kept job checkout %s: %v\n", worktree, err)
	}
	return runErr
}

func removeFinishedJobCheckout(ctx context.Context, workspace, home string, job storage.WorktreeRegistration) error {
	unlock, err := filelock.Lock(filepath.Join(home, "cleanup.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	released := filepath.Join(home, "released")
	if _, err := os.Lstat(released); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(job.Path); os.IsNotExist(err) {
		return os.WriteFile(released, nil, 0o600)
	} else if err != nil {
		return err
	}
	if job.MergedHead != "" {
		if checkoutPath(workspace, job.Path) != job.Path || IsJobDirectory(workspace, job.Path) {
			return fmt.Errorf("adopted checkout is outside the workspace worktrees")
		}
		info := gitinfo.Inspect(ctx, job.Path)
		if info.Head != job.MergedHead || info.Ahead == nil || *info.Ahead != 0 || info.Dirty == nil || *info.Dirty {
			return fmt.Errorf("adopted checkout changed")
		}
	} else if job.Path != filepath.Join(home, ".worktrees", filepath.Base(home)) {
		return fmt.Errorf("job checkout path does not match its registration")
	}
	if err := gitinfo.CheckWorktree(ctx, job.Path, job.Project); err != nil {
		return err
	}
	if err := gitinfo.RemoveManagedWorktree(ctx, job.Path, job.Branch, "Save worktree job changes"); err != nil {
		return err
	}
	return os.WriteFile(released, nil, 0o600)
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
		finished, err := os.Lstat(filepath.Join(home, "finished"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !finished.Mode().IsRegular() {
			continue
		}
		job, err := readRegistration(home)
		if err == nil {
			err = sweepRegisteredJob(ctx, workspace, home, job, store)
		}
		if err != nil {
			logger.Warn("kept job checkout", "path", home, "error", err)
			continue
		}
		caches = append(caches, jobCache{path: filepath.Join(home, "cache"), finished: finished.ModTime()})
	}
	return trimJobCaches(ctx, root, caches, jobCacheBudget, minimumFree)
}

func sweepRegisteredJob(ctx context.Context, workspace, home string, job storage.WorktreeRegistration, store sessionReader) error {
	if job.MergedHead != "" {
		defer LockCheckout(workspace, job.Path)()
		groups, err := managedDirectories(store, workspace)
		if err != nil {
			return err
		}
		if groups[job.Path] != nil {
			return fmt.Errorf("adopted checkout has a thread owner")
		}
	}
	return removeFinishedJobCheckout(ctx, workspace, home, job)
}
