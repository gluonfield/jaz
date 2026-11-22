package plugins

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
)

type MarketplaceEntry struct {
	ID               string   `json:"id"`
	DisplayName      string   `json:"display_name"`
	Description      string   `json:"description,omitempty"`
	Version          string   `json:"version,omitempty"`
	Category         string   `json:"category,omitempty"`
	Author           string   `json:"author,omitempty"`
	Homepage         string   `json:"homepage,omitempty"`
	Repository       string   `json:"repository,omitempty"`
	Ref              string   `json:"ref,omitempty"`
	Path             string   `json:"path,omitempty"`
	IconDataURL      string   `json:"icon_data_url,omitempty"`
	SuggestedPrompts []string `json:"suggested_prompts,omitempty"`
	Installation     string   `json:"installation,omitempty"`
	Authentication   string   `json:"authentication,omitempty"`
	Execution        string   `json:"execution,omitempty"`
	Privacy          string   `json:"privacy,omitempty"`
	SourceRepo       string   `json:"source_repo,omitempty"`
	HasRemoteMCP     bool     `json:"has_remote_mcp,omitempty"`
}

var marketplacePaths = []string{
	".agents/plugins/marketplace.json",
	".claude-plugin/marketplace.json",
	".cursor-plugin/marketplace.json",
}

type rawMarketplace struct {
	Name    string `json:"name"`
	Plugins []struct {
		Name        string          `json:"name"`
		DisplayName string          `json:"displayName"`
		Description string          `json:"description"`
		Version     string          `json:"version"`
		Category    string          `json:"category"`
		Homepage    string          `json:"homepage"`
		Repository  string          `json:"repository"`
		Source      json.RawMessage `json:"source"`
		Policy      struct {
			Installation   string `json:"installation"`
			Authentication string `json:"authentication"`
		} `json:"policy"`
		Execution string `json:"execution"`
		Privacy   string `json:"privacy"`
	} `json:"plugins"`
}

type parsedEntrySource struct {
	Kind       string
	Path       string
	Repository string
	Ref        string
}

func parseMarketplaceSource(raw json.RawMessage) (parsedEntrySource, bool) {
	var relPath string
	if json.Unmarshal(raw, &relPath) == nil && relPath != "" {
		return parsedEntrySource{Kind: "local", Path: relPath}, true
	}
	var obj struct {
		Source string `json:"source"`
		Path   string `json:"path"`
		URL    string `json:"url"`
		Repo   string `json:"repo"`
		Ref    string `json:"ref"`
		SHA    string `json:"sha"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return parsedEntrySource{}, false
	}
	ref := cmp.Or(strings.TrimSpace(obj.SHA), strings.TrimSpace(obj.Ref))
	switch strings.ToLower(strings.TrimSpace(obj.Source)) {
	case "local":
		return parsedEntrySource{Kind: "local", Path: obj.Path}, obj.Path != ""
	case "git", "git-subdir":
		repo := cmp.Or(strings.TrimSpace(obj.URL), strings.TrimSpace(obj.Repo))
		return parsedEntrySource{Kind: "git", Repository: repo, Path: obj.Path, Ref: ref}, repo != ""
	case "github":
		repo := cmp.Or(strings.TrimSpace(obj.Repo), strings.TrimSpace(obj.URL))
		return parsedEntrySource{Kind: "git", Repository: repo, Path: obj.Path, Ref: ref}, repo != ""
	default:
		return parsedEntrySource{}, false
	}
}

func (s *Store) marketplace(repoRoot, repoLabel string) ([]preparedPlugin, []MarketplaceEntry, []SkippedPlugin, bool, error) {
	var raw []byte
	for _, rel := range marketplacePaths {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, nil, nil, false, err
		}
		raw = data
		break
	}
	if raw == nil {
		prepared, err := s.discover(repoRoot, repoRoot)
		if err != nil {
			return nil, nil, nil, false, err
		}
		entries := make([]MarketplaceEntry, 0, len(prepared))
		for _, item := range prepared {
			entries = append(entries, entryFromPlugin(item.plugin, repoLabel, "", ""))
		}
		return prepared, entries, nil, false, nil
	}
	var doc rawMarketplace
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, nil, true, fmt.Errorf("%w: invalid marketplace JSON", ErrInvalid)
	}
	if doc.Plugins == nil {
		return nil, nil, nil, true, fmt.Errorf("%w: marketplace needs a plugins array", ErrInvalid)
	}
	siblings := scanSiblingFlavors(repoRoot, s.dataDir)
	var prepared []preparedPlugin
	var entries []MarketplaceEntry
	var skipped []SkippedPlugin
	for _, item := range doc.Plugins {
		if item.Name == "" || !manifest.SafeName(item.Name) {
			return nil, nil, nil, true, fmt.Errorf("%w: invalid marketplace plugin name %q", ErrInvalid, item.Name)
		}
		src, ok := parseMarketplaceSource(item.Source)
		if !ok {
			skipped = append(skipped, SkippedPlugin{Name: item.Name, Reason: "unsupported marketplace source"})
			continue
		}
		if src.Kind == "git" {
			entries = append(entries, MarketplaceEntry{
				ID:             item.Name,
				DisplayName:    cmp.Or(item.DisplayName, item.Name),
				Description:    item.Description,
				Version:        item.Version,
				Category:       item.Category,
				Homepage:       item.Homepage,
				Repository:     src.Repository,
				Ref:            src.Ref,
				Path:           src.Path,
				Installation:   item.Policy.Installation,
				Authentication: item.Policy.Authentication,
				Execution:      item.Execution,
				Privacy:        item.Privacy,
				SourceRepo:     repoLabel,
				HasRemoteMCP:   strings.EqualFold(item.Policy.Authentication, "ON_USE"),
			})
			continue
		}
		resolved, err := manifest.Resolve(repoRoot, src.Path)
		if err != nil {
			skipped = append(skipped, SkippedPlugin{Name: item.Name, Reason: "local source must stay inside the repository"})
			continue
		}
		plugin, err := readPackage(resolved, s.dataDir, nil)
		if err != nil || plugin.ID != item.Name {
			skipped = append(skipped, SkippedPlugin{Name: item.Name, Reason: "local source has no valid matching plugin manifest"})
			continue
		}
		if extra, found := siblings[item.Name]; found {
			plugin = mergePluginMetadata(plugin, extra)
		}
		if item.Description != "" && plugin.Description == "" {
			plugin.Description = item.Description
		}
		if item.Category != "" && plugin.Category == "" {
			plugin.Category = item.Category
		}
		if item.Homepage != "" && plugin.Homepage == "" {
			plugin.Homepage = item.Homepage
		}
		if item.Repository != "" && plugin.Repository == "" {
			plugin.Repository = item.Repository
		}
		relPath, _ := filepath.Rel(repoRoot, resolved)
		relPath = filepath.ToSlash(relPath)
		if relPath == "." {
			relPath = ""
		}
		entry := entryFromPlugin(plugin, repoLabel, relPath, "")
		entry.Installation = item.Policy.Installation
		entry.Authentication = item.Policy.Authentication
		entry.Execution = item.Execution
		entry.Privacy = item.Privacy
		entries = append(entries, entry)
		prepared = append(prepared, preparedPlugin{plugin: plugin, path: resolved})
	}
	if len(prepared) == 0 && len(entries) == 0 && len(skipped) == 0 {
		return nil, nil, nil, true, fmt.Errorf("%w: marketplace contains no plugins", ErrInvalid)
	}
	return prepared, entries, skipped, true, nil
}

// scanSiblingFlavors reads alternative flavor directories (e.g. antigravity/<name>)
// in the same repository so logos and suggestedPrompts enrich the canonical entry.
func scanSiblingFlavors(repoRoot, dataDir string) map[string]Plugin {
	out := map[string]Plugin{}
	for _, parent := range []string{"antigravity", "plugins"} {
		dir := filepath.Join(repoRoot, parent)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			plugin, err := readPackage(filepath.Join(dir, entry.Name()), dataDir, nil)
			if err != nil {
				continue
			}
			if existing, ok := out[plugin.ID]; ok {
				out[plugin.ID] = mergePluginMetadata(existing, plugin)
			} else {
				out[plugin.ID] = plugin
			}
		}
	}
	return out
}

func entryFromPlugin(plugin Plugin, repoLabel, relPath, ref string) MarketplaceEntry {
	hasRemote := false
	for _, srv := range plugin.MCPServers {
		if srv.Transport == string(mcpconfig.TransportStreamableHTTP) && srv.URL != "" {
			hasRemote = true
			break
		}
	}
	return MarketplaceEntry{
		ID:               plugin.ID,
		DisplayName:      plugin.DisplayName,
		Description:      plugin.Description,
		Version:          plugin.Version,
		Category:         plugin.Category,
		Author:           plugin.Author,
		Homepage:         plugin.Homepage,
		Repository:       cmp.Or(repoLabel, plugin.Repository),
		Ref:              ref,
		Path:             relPath,
		IconDataURL:      plugin.IconDataURL,
		SuggestedPrompts: plugin.SuggestedPrompts,
		SourceRepo:       repoLabel,
		HasRemoteMCP:     hasRemote,
	}
}
