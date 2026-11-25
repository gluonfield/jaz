package worktrees

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	jobCacheBudget = 8 << 30
	minimumFree    = 15 << 30
)

type jobCache struct {
	path     string
	finished time.Time
	bytes    uint64
}

func trimJobCaches(ctx context.Context, root string, caches []jobCache, budget, freeFloor uint64) error {
	if len(caches) == 0 {
		return nil
	}
	free, err := freeBytes(root)
	if err != nil {
		return err
	}
	var total uint64
	for i := range caches {
		caches[i].bytes, err = cacheBytes(ctx, caches[i].path)
		if err != nil {
			return err
		}
		total += caches[i].bytes
	}
	slices.SortFunc(caches, func(a, b jobCache) int { return a.finished.Compare(b.finished) })
	for _, cache := range caches {
		if total <= budget && free >= freeFloor {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if cache.bytes == 0 {
			continue
		}
		if err := os.RemoveAll(cache.path); err != nil {
			return err
		}
		total -= cache.bytes
		free, err = freeBytes(root)
		if err != nil {
			return err
		}
	}
	return nil
}

func cacheBytes(ctx context.Context, dir string) (uint64, error) {
	var total uint64
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if path == dir && os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += uint64(info.Size())
		return nil
	})
	return total, err
}
