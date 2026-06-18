package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var installMu sync.Mutex

func InstallMissingTo(root, dstRoot string) error {
	installMu.Lock()
	defer installMu.Unlock()

	catalog, err := Load(root)
	if err != nil {
		return err
	}
	dstRoot = strings.TrimSpace(dstRoot)
	if dstRoot == "" || len(catalog.Skills) == 0 {
		return nil
	}
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		return err
	}
	for _, skill := range catalog.Skills {
		if err := installMissingSkill(filepath.Dir(skill.Path), filepath.Join(dstRoot, filepath.Base(filepath.Dir(skill.Path)))); err != nil {
			return err
		}
	}
	return nil
}

func installMissingSkill(src, dst string) error {
	if err := os.Mkdir(dst, 0o755); err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	if err := copyDir(src, dst); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return nil
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
