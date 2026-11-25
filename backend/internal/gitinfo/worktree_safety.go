package gitinfo

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func CheckWorktree(ctx context.Context, dir, project string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree path %s is not a directory", dir)
	}
	root, _, linked := mainCheckout(ctx, dir)
	expected, err := filepath.EvalSymlinks(project)
	if err != nil {
		return err
	}
	if !linked || root != expected {
		return fmt.Errorf("checkout at %s does not belong to %s", dir, project)
	}
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	if top != resolved {
		return fmt.Errorf("%s is not the checkout root", dir)
	}
	return nil
}

func StampWorktreeAdoption(ctx context.Context, dir, ticket string) error {
	path, err := adoptionStampPath(ctx, dir, ticket)
	if err != nil {
		return err
	}
	stamp, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	return stamp.Close()
}

func CheckWorktreeAdoption(ctx context.Context, dir, ticket string) error {
	path, err := adoptionStampPath(ctx, dir, ticket)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("checkout has no matching adoption stamp: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("checkout adoption stamp is not a regular file")
	}
	return nil
}

func adoptionStampPath(ctx context.Context, dir, ticket string) (string, error) {
	return git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-path", "jaz-adopt-"+ticket)
}

func checkNestedGit(dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == ".git" && filepath.Dir(path) != dir {
			return fmt.Errorf("worktree contains a nested repository at %s", filepath.Dir(path))
		}
		if entry.Name() == "HEAD" && entry.Type().IsRegular() {
			parent := filepath.Dir(path)
			if objects, oerr := os.Stat(filepath.Join(parent, "objects")); oerr == nil && objects.IsDir() {
				if refs, rerr := os.Stat(filepath.Join(parent, "refs")); rerr == nil && refs.IsDir() {
					return fmt.Errorf("worktree contains a bare repository at %s", parent)
				}
			}
		}
		return nil
	})
}
