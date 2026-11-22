package skills

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/templates/skillsprompt"
)

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Plugin      string `json:"plugin,omitempty"`
}

type Catalog struct {
	Root   string
	Skills []Skill
}

const (
	userSkillsDir = "skills"
	pluginsDir    = "plugins"
)

var workspaceSkillDirs = []string{".claude", ".codex", ".agents", ".jaz"}

func UserRoot(root string) string {
	return filepath.Join(root, userSkillsDir)
}

func Load(root string) (Catalog, error) {
	return LoadFiltered(root, nil)
}

func LoadFiltered(root string, allowPlugin func(string) bool) (Catalog, error) {
	if root == "" {
		return Catalog{}, nil
	}
	userRoot := UserRoot(root)
	out, err := LoadDir(userRoot)
	if err != nil {
		return Catalog{}, err
	}
	entries, err := os.ReadDir(filepath.Join(root, pluginsDir))
	if err != nil && !os.IsNotExist(err) {
		return Catalog{}, err
	}
	var packaged []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if allowPlugin != nil && !allowPlugin(entry.Name()) {
			continue
		}
		pluginRoot := filepath.Join(root, pluginsDir, entry.Name())
		plugin, err := manifest.Read(pluginRoot)
		if err != nil {
			if !errors.Is(err, manifest.ErrNotPlugin) {
				continue
			}
			held, _ := LoadPlugin(pluginRoot, entry.Name(), nil)
			packaged = mergeSkills(packaged, held)
			continue
		}
		if plugin.Name != entry.Name() {
			continue
		}
		held, _ := LoadPlugin(pluginRoot, plugin.Name, plugin.ExtraSkillPaths)
		packaged = mergeSkills(packaged, held)
	}
	return Catalog{Root: userRoot, Skills: mergeSkills(packaged, out)}, nil
}

func LoadForWorkspace(root, workspace string) (Catalog, error) {
	return LoadForWorkspaceFiltered(root, workspace, nil)
}

func LoadForWorkspaceFiltered(root, workspace string, allowPlugin func(string) bool) (Catalog, error) {
	catalog, err := LoadFiltered(root, allowPlugin)
	if err != nil {
		return Catalog{}, err
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return catalog, nil
	}
	for _, dir := range workspaceSkillDirs {
		local, err := LoadDir(filepath.Join(workspace, dir, userSkillsDir))
		if err != nil {
			return Catalog{}, err
		}
		catalog.Skills = mergeSkills(catalog.Skills, local)
	}
	return catalog, nil
}

// LoadDir returns every skill under a folder, at any depth.
func LoadDir(skillsRoot string) ([]Skill, error) {
	info, err := os.Stat(skillsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
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
		return nil, err
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
	return out, nil
}

func mergeSkills(base, overlay []Skill) []Skill {
	if len(overlay) == 0 {
		return base
	}
	out := append([]Skill(nil), base...)
	seen := make(map[string]int, len(out)+len(overlay))
	for i, skill := range out {
		seen[strings.ToLower(skill.Name)] = i
	}
	for _, skill := range overlay {
		key := strings.ToLower(skill.Name)
		if i, ok := seen[key]; ok {
			out[i] = skill
			continue
		}
		seen[key] = len(out)
		out = append(out, skill)
	}
	return out
}

func (c Catalog) Prompt() string {
	if len(c.Skills) == 0 {
		return ""
	}
	data := skillsprompt.Data{Skills: make([]skillsprompt.Skill, 0, len(c.Skills))}
	for _, skill := range c.Skills {
		data.Skills = append(data.Skills, skillsprompt.Skill{
			Name:        skill.Name,
			Description: skill.Description,
			Location:    skill.Path,
		})
	}
	prompt, err := skillsprompt.Render(data)
	if err != nil {
		// Embedded and parse-checked at init; execution cannot realistically fail.
		return ""
	}
	return prompt
}

func shouldSkipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
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
