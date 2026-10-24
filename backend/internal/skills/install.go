package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

var installMu sync.Mutex

func replaceSkill(src, dst string) error {
	tmp, err := os.MkdirTemp(filepath.Dir(dst), "."+filepath.Base(dst)+"-install-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := copyDir(src, tmp); err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("copy skill file %s: %w", path, err)
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
