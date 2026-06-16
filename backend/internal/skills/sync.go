package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const managedMarker = ".jaz-managed-skill"

func SyncTo(root, dstRoot string) error {
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
		if err := syncSkill(filepath.Dir(skill.Path), filepath.Join(dstRoot, filepath.Base(filepath.Dir(skill.Path)))); err != nil {
			return err
		}
	}
	return nil
}

func syncSkill(src, dst string) error {
	if info, err := os.Stat(dst); err == nil {
		if !info.IsDir() || !exists(filepath.Join(dst, managedMarker)) {
			return nil
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	tmp, err := os.MkdirTemp(filepath.Dir(dst), ".jaz-skill-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := copyDir(src, tmp); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, managedMarker), []byte("managed by Jaz\n"), 0o644); err != nil {
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

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
