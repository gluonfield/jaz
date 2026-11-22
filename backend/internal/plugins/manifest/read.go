package manifest

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var candidatePaths = []string{
	"plugin.json",
	".codex-plugin/plugin.json",
	".claude-plugin/plugin.json",
	".cursor-plugin/plugin.json",
	".antigravity-plugin/plugin.json",
	".gemini-plugin/plugin.json",
}

func Read(dir string) (Manifest, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Manifest{}, err
	}
	defer root.Close()
	var primary Manifest
	found := false
	for _, rel := range candidatePaths {
		data, err := root.ReadFile(rel)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			if !found {
				return Manifest{}, err
			}
			continue
		}
		parsed, err := parseManifest(data, rel, filepath.Base(dir))
		if err != nil {
			if !found {
				return Manifest{}, fmt.Errorf("%s: %w", rel, err)
			}
			continue
		}
		if !found {
			primary = parsed
			found = true
			continue
		}
		if parsed.Name == primary.Name {
			primary = mergeMetadata(primary, parsed)
		}
	}
	if !found {
		return Manifest{}, ErrNotPlugin
	}
	if !SafeName(primary.Name) {
		return Manifest{}, fmt.Errorf("%s: invalid plugin name", primary.ManifestPath)
	}
	if primary.DisplayName == "" {
		primary.DisplayName = primary.Name
	}
	return primary, nil
}

func ReadClaude(dir string) (Manifest, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Manifest{}, err
	}
	defer root.Close()
	data, err := root.ReadFile(".claude-plugin/plugin.json")
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{Name: filepath.Base(dir)}, nil
	}
	if err != nil {
		return Manifest{}, err
	}
	plugin, err := parseManifest(data, ".claude-plugin/plugin.json", filepath.Base(dir))
	if err == nil && !SafeName(plugin.Name) {
		err = errors.New("invalid Claude plugin name")
	}
	return plugin, err
}

func MergeMetadata(base, extra Manifest) Manifest {
	return mergeMetadata(base, extra)
}

func mergeMetadata(base, extra Manifest) Manifest {
	base.DisplayName = cmp.Or(base.DisplayName, extra.DisplayName)
	base.Description = cmp.Or(base.Description, extra.Description)
	base.Version = cmp.Or(base.Version, extra.Version)
	base.Category = cmp.Or(base.Category, extra.Category)
	base.Author = cmp.Or(base.Author, extra.Author)
	base.AuthorURL = cmp.Or(base.AuthorURL, extra.AuthorURL)
	base.Homepage = cmp.Or(base.Homepage, extra.Homepage)
	base.Repository = cmp.Or(base.Repository, extra.Repository)
	base.IconPath = cmp.Or(base.IconPath, extra.IconPath)
	if len(base.Keywords) == 0 && len(extra.Keywords) > 0 {
		base.Keywords = extra.Keywords
	}
	if len(base.SuggestedPrompts) == 0 && len(extra.SuggestedPrompts) > 0 {
		base.SuggestedPrompts = extra.SuggestedPrompts
	}
	if base.Page == nil && extra.Page != nil {
		base.Page = extra.Page
	}
	return base
}

type stringList []string

func (s *stringList) UnmarshalJSON(data []byte) error {
	var single string
	if json.Unmarshal(data, &single) == nil {
		if single != "" {
			*s = []string{single}
		}
		return nil
	}
	var multiple []string
	if err := json.Unmarshal(data, &multiple); err != nil {
		return err
	}
	*s = multiple
	return nil
}

type authorField struct {
	Name string
	URL  string
}

func (a *authorField) UnmarshalJSON(data []byte) error {
	var name string
	if json.Unmarshal(data, &name) == nil {
		a.Name = strings.TrimSpace(name)
		return nil
	}
	var obj struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	a.Name = strings.TrimSpace(obj.Name)
	a.URL = strings.TrimSpace(obj.URL)
	return nil
}

type repoField string

func (r *repoField) UnmarshalJSON(data []byte) error {
	var raw string
	if json.Unmarshal(data, &raw) == nil {
		*r = repoField(strings.TrimSpace(raw))
		return nil
	}
	var obj struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	*r = repoField(strings.TrimSpace(obj.URL))
	return nil
}

type rawManifest struct {
	Name             string          `json:"name"`
	DisplayName      string          `json:"displayName"`
	Description      string          `json:"description"`
	Version          string          `json:"version"`
	Category         stringList      `json:"category"`
	Author           authorField     `json:"author"`
	Homepage         string          `json:"homepage"`
	Repository       repoField       `json:"repository"`
	Keywords         []string        `json:"keywords"`
	Tags             []string        `json:"tags"`
	Logo             string          `json:"logo"`
	Icon             string          `json:"icon"`
	SuggestedPrompts stringList      `json:"suggestedPrompts"`
	Examples         stringList      `json:"examples"`
	Skills           stringList      `json:"skills"`
	MCPServers       json.RawMessage `json:"mcpServers"`
	Page             *Page           `json:"page"`
	Interface        struct {
		DisplayName      string     `json:"displayName"`
		ShortDescription string     `json:"shortDescription"`
		LongDescription  string     `json:"longDescription"`
		DeveloperName    string     `json:"developerName"`
		Category         stringList `json:"category"`
		WebsiteURL       string     `json:"websiteURL"`
		Logo             string     `json:"logo"`
		DefaultPrompt    stringList `json:"defaultPrompt"`
	} `json:"interface"`
}

func parseManifest(data []byte, rel, fallbackName string) (Manifest, error) {
	var raw rawManifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, err
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" && rel == "plugin.json" {
		name = fallbackName
	}
	if name == "" {
		return Manifest{}, errors.New("plugin name is required")
	}
	category := firstString(raw.Category)
	if category == "" {
		category = firstString(raw.Interface.Category)
	}
	keywords := raw.Keywords
	if len(keywords) == 0 {
		keywords = raw.Tags
	}
	prompts := []string(raw.SuggestedPrompts)
	if len(prompts) == 0 {
		prompts = []string(raw.Examples)
	}
	if len(prompts) == 0 {
		prompts = []string(raw.Interface.DefaultPrompt)
	}
	var page *Page
	if raw.Page != nil && strings.TrimSpace(raw.Page.View) != "" {
		page = &Page{View: strings.TrimSpace(raw.Page.View)}
	}
	return Manifest{
		Name:             name,
		DisplayName:      cmp.Or(strings.TrimSpace(raw.DisplayName), strings.TrimSpace(raw.Interface.DisplayName)),
		Description:      cmp.Or(strings.TrimSpace(raw.Description), strings.TrimSpace(raw.Interface.ShortDescription), strings.TrimSpace(raw.Interface.LongDescription)),
		Version:          strings.TrimSpace(raw.Version),
		Category:         strings.TrimSpace(category),
		Author:           cmp.Or(raw.Author.Name, strings.TrimSpace(raw.Interface.DeveloperName)),
		AuthorURL:        raw.Author.URL,
		Homepage:         cmp.Or(strings.TrimSpace(raw.Homepage), strings.TrimSpace(raw.Interface.WebsiteURL)),
		Repository:       string(raw.Repository),
		Keywords:         keywords,
		IconPath:         cmp.Or(strings.TrimSpace(raw.Logo), strings.TrimSpace(raw.Icon), strings.TrimSpace(raw.Interface.Logo)),
		SuggestedPrompts: prompts,
		Page:             page,
		ManifestPath:     rel,
		ExtraSkillPaths:  []string(raw.Skills),
		MCPServers:       raw.MCPServers,
	}, nil
}

func firstString(items []string) string {
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
