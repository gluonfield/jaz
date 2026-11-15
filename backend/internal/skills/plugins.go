package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
)

func LoadPlugin(root string, plugin manifest.Plugin) ([]Skill, []string) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, []string{err.Error()}
	}
	var out []Skill
	var warnings []string
	seen := map[string]bool{}
	add := func(path string) {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return
		}
		path, err = manifest.Resolve(root, "./"+filepath.ToSlash(relative))
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("Skipping skill %s: %v", relative, err))
			}
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || seen[path] {
			return
		}
		seen[path] = true
		if skill, ok := readSkill(path); ok {
			skill.Plugin = plugin.Name
			out = append(out, skill)
		} else {
			warnings = append(warnings, "Skipping invalid skill "+relative)
		}
	}
	for i, source := range plugin.SkillDirs {
		directory, err := manifest.Resolve(root, source)
		if err != nil {
			if !os.IsNotExist(err) || i > 0 {
				warnings = append(warnings, fmt.Sprintf("Skipping skills path %s: %v", source, err))
			}
			continue
		}
		if i > 0 {
			if info, err := os.Stat(filepath.Join(directory, "SKILL.md")); err == nil && info.Mode().IsRegular() {
				add(filepath.Join(directory, "SKILL.md"))
				continue
			}
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Skipping skills path %s: %v", source, err))
			continue
		}
		for _, entry := range entries {
			child := filepath.Join(directory, entry.Name())
			if info, err := os.Stat(child); err == nil && info.IsDir() {
				add(filepath.Join(child, "SKILL.md"))
			}
		}
	}
	return out, warnings
}
