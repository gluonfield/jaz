package skills

import (
	"crypto/sha256"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
)

func QuarantineProfileCopies(root, profile string) error {
	installMu.Lock()
	defer installMu.Unlock()
	for _, path := range []string{profile, filepath.Join(profile, "skills")} {
		if info, err := os.Lstat(path); err != nil || !info.IsDir() {
			return nil
		}
	}
	catalog, err := Load(root)
	if err != nil {
		return err
	}
	for _, skill := range catalog.Skills {
		source := filepath.Dir(skill.Path)
		copy := filepath.Join(profile, "skills", filepath.Base(source))
		if info, err := os.Lstat(copy); err != nil || !info.IsDir() {
			continue
		}
		original, err := skillFiles(source)
		if err != nil {
			continue
		}
		copied, err := skillFiles(copy)
		if err != nil || !maps.Equal(original, copied) {
			continue
		}
		backup := filepath.Join(profile, "retired-jaz-skills", filepath.Base(source))
		if _, err := os.Lstat(backup); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
			return err
		}
		if info, err := os.Lstat(filepath.Dir(backup)); err != nil || !info.IsDir() {
			return fs.ErrInvalid
		}
		if err := os.Rename(copy, backup); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

type skillFile struct {
	Mode fs.FileMode
	Hash [sha256.Size]byte
}

func skillFiles(root string) (map[string]skillFile, error) {
	files := map[string]skillFile{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file := skillFile{Mode: info.Mode()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file.Hash = sha256.Sum256(data)
		} else if !entry.IsDir() {
			return fs.ErrInvalid
		}
		files[rel] = file
		return nil
	})
	return files, err
}
