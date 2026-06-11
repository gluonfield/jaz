package skills

import (
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

type Catalog struct {
	Root   string
	Skills []Skill
}

func Load(root string) (Catalog, error) {
	if root == "" {
		return Catalog{}, nil
	}
	skillsRoot := filepath.Join(root, "skills")
	info, err := os.Stat(skillsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return Catalog{Root: skillsRoot}, nil
		}
		return Catalog{}, err
	}
	if !info.IsDir() {
		return Catalog{Root: skillsRoot}, nil
	}

	var paths []string
	if err := filepath.WalkDir(skillsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != skillsRoot && shouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return Catalog{}, err
	}
	sort.Strings(paths)

	seen := make(map[string]struct{}, len(paths))
	out := make([]Skill, 0, len(paths))
	for _, path := range paths {
		skill, ok := readSkill(path)
		if !ok {
			continue
		}
		key := strings.ToLower(skill.Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, skill)
	}
	return Catalog{Root: skillsRoot, Skills: out}, nil
}

func (c Catalog) Prompt() string {
	if len(c.Skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Skills\n")
	b.WriteString("Use a listed skill when named or when its description matches the task. Users may reference a skill inline as $name in their messages. Read its SKILL.md first; resolve relative paths from that file; load extras only as needed.\n\n")
	b.WriteString("<available_skills>\n")
	for _, skill := range c.Skills {
		fmt.Fprintf(&b, "  <skill>\n    <name>%s</name>\n    <description>%s</description>\n    <location>%s</location>\n  </skill>\n", html.EscapeString(skill.Name), html.EscapeString(skill.Description), html.EscapeString(skill.Path))
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func shouldSkipDir(name string) bool {
	return name == ".git" || name == ".archive" || name == "node_modules"
}

func readSkill(path string) (Skill, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, false
	}
	name, description := parseFrontmatter(string(data))
	if name == "" || description == "" {
		return Skill{}, false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Skill{}, false
	}
	return Skill{Name: name, Description: description, Path: abs}, true
}

func parseFrontmatter(content string) (string, string) {
	content = strings.TrimPrefix(content, "\ufeff")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	var name, description string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = trimScalar(value)
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description
}

func trimScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}
