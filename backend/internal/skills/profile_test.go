package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuarantineProfileCopiesPreservesCustomSkillsAndBackups(t *testing.T) {
	root, profile := t.TempDir(), t.TempDir()
	for _, directory := range []string{root, profile} {
		for _, name := range []string{"exact", "edited", "extra", "backup", "linked"} {
			writeSkill(t, directory, name, name, "a skill")
			writeFile(t, filepath.Join(directory, "skills", name, "run.py"), "original")
		}
	}
	writeFile(t, filepath.Join(profile, "skills", "edited", "run.py"), "custom")
	writeFile(t, filepath.Join(profile, "skills", "extra", "notes.md"), "keep")
	writeFile(t, filepath.Join(profile, "retired-jaz-skills", "backup", "notes.md"), "previous backup")
	if err := os.RemoveAll(filepath.Join(profile, "skills", "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "skills", "linked"), filepath.Join(profile, "skills", "linked")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := QuarantineProfileCopies(root, profile); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(profile, "skills", "exact")); !os.IsNotExist(err) {
		t.Fatalf("exact copy remains discoverable: %v", err)
	}
	for path, want := range map[string]string{
		"retired-jaz-skills/exact/run.py":    "original",
		"skills/edited/run.py":               "custom",
		"skills/extra/notes.md":              "keep",
		"skills/backup/run.py":               "original",
		"retired-jaz-skills/backup/notes.md": "previous backup",
		"skills/linked/run.py":               "original",
	} {
		if data, err := os.ReadFile(filepath.Join(profile, path)); err != nil || string(data) != want {
			t.Fatalf("%s = %q, %v", path, data, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(root, "skills", "exact", "run.py")); err != nil || string(data) != "original" {
		t.Fatalf("canonical source changed: %q, %v", data, err)
	}
}

func TestQuarantineProfileCopiesDoesNotFollowSkillDirectorySymlinks(t *testing.T) {
	root, profile := t.TempDir(), t.TempDir()
	writeSkill(t, root, "exact", "exact", "a skill")
	if err := os.Symlink(filepath.Join(root, "skills"), filepath.Join(profile, "skills")); err != nil {
		t.Fatal(err)
	}
	if err := QuarantineProfileCopies(root, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "exact", "SKILL.md")); err != nil {
		t.Fatalf("canonical source moved through symlink: %v", err)
	}
}

func TestQuarantineProfileCopiesDoesNotFollowBackupSymlinks(t *testing.T) {
	root, profile, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeSkill(t, root, "exact", "exact", "a skill")
	writeSkill(t, profile, "exact", "exact", "a skill")
	if err := os.Symlink(outside, filepath.Join(profile, "retired-jaz-skills")); err != nil {
		t.Fatal(err)
	}
	if err := QuarantineProfileCopies(root, profile); err == nil {
		t.Fatal("accepted a symlinked backup directory")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("wrote through backup symlink: %v, %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(profile, "skills", "exact", "SKILL.md")); err != nil {
		t.Fatalf("active skill moved through symlink: %v", err)
	}
}
