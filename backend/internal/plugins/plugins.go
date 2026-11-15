// Package plugins keeps installed plugins. A plugin is a folder with a
// plugin.json, skills under skills/ and HTML views under views/.
package plugins

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/skills"
)

var (
	ErrNotFound = errors.New("plugin not found")
	ErrInvalid  = errors.New("invalid plugin")
	name        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

type Plugin struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Skills      []string `json:"skills"`
	Views       []string `json:"views"`
	MCPServers  []string `json:"mcp_servers"`
	IconDataURL string   `json:"icon_data_url,omitempty"`
	Page        *Page    `json:"page,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	Source      *Source  `json:"source,omitempty"`
}

type Store struct {
	dir         string
	dataDir     string
	mu          sync.RWMutex
	sourcesPath string
}

func New(layout runtimefiles.Layout) *Store {
	return &Store{dir: layout.Plugins, dataDir: filepath.Join(layout.Root, "plugin-data"), sourcesPath: filepath.Join(layout.Root, "plugin-sources.json")}
}

func (s *Store) List() ([]Plugin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Plugin{}, nil
	}
	if err != nil {
		return nil, err
	}
	sources, err := s.sources()
	if err != nil {
		return nil, err
	}
	out := []Plugin{}
	for _, entry := range entries {
		plugin, err := read(filepath.Join(s.dir, entry.Name()), s.dataDir)
		if err == nil && plugin.Name == entry.Name() {
			if source, ok := sources[plugin.Name]; ok {
				plugin.Source = &source
			}
			out = append(out, plugin)
		}
	}
	return out, nil
}

// View returns a view's HTML by its reference, "<plugin>/<view>".
func (s *Store) View(ref string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.view(ref)
}

func (s *Store) view(ref string) (string, error) {
	plugin, view, ok := strings.Cut(ref, "/")
	if !ok || !manifest.SafeName(plugin) || !name.MatchString(view) {
		return "", fmt.Errorf("%w: view %q is not <plugin>/<view>", ErrInvalid, ref)
	}
	root, err := os.OpenRoot(filepath.Join(s.dir, plugin))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	defer root.Close()
	html, err := root.ReadFile("views/" + view + ".html")
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: view %q", ErrNotFound, ref)
	}
	return string(html), err
}

// Install adds every plugin found in a zip, at any depth, replacing an
// installed plugin of the same name. A repository's downloaded zip installs
// all the plugins it holds.
func (s *Store) Install(archive []byte) (installed []Plugin, err error) {
	if err = os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(s.dir, ".install-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	tree := filepath.Join(work, "content")
	if err = os.MkdirAll(tree, 0755); err != nil {
		return nil, err
	}
	if err = skills.ExtractArchive(archive, tree); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	prepared, err := s.discover(tree)
	if err != nil {
		return nil, err
	}
	prepared, err = stagePackages(prepared, filepath.Join(work, "ready"))
	if err != nil {
		return nil, err
	}
	return s.publish(prepared)
}

func (s *Store) Remove(plugin string) error {
	if !manifest.SafeName(plugin) {
		return fmt.Errorf("%w: name %q", ErrInvalid, plugin)
	}
	_, err := s.publish([]preparedPlugin{{plugin: Plugin{Name: plugin}}})
	return err
}

func read(dir, dataDir string) (Plugin, error) {
	definition, err := manifest.Read(dir)
	if err != nil {
		return Plugin{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	plugin := Plugin{Name: definition.Name, DisplayName: definition.DisplayName, Description: definition.Description, Version: definition.Version, Warnings: definition.Warnings}
	if definition.Page != nil {
		plugin.Page = &Page{View: definition.Page.View}
	}
	held, warnings := skills.LoadPlugin(dir, definition.Name, definition.ExtraSkillPaths)
	plugin.Warnings = append(plugin.Warnings, warnings...)
	plugin.Skills = make([]string, len(held))
	for i, skill := range held {
		plugin.Skills[i] = skill.Name
	}
	views, err := filepath.Glob(filepath.Join(dir, "views", "*.html"))
	if err != nil {
		return Plugin{}, err
	}
	plugin.Views = make([]string, len(views))
	for i, view := range views {
		plugin.Views[i] = strings.TrimSuffix(filepath.Base(view), ".html")
	}
	if plugin.Page != nil {
		if !name.MatchString(plugin.Page.View) || !slices.Contains(plugin.Views, plugin.Page.View) {
			return Plugin{}, fmt.Errorf("%w: page view does not exist", ErrInvalid)
		}
	}
	plugin.IconDataURL, err = readIcon(dir, definition.IconPath)
	if err != nil {
		return Plugin{}, err
	}
	servers, warnings := loadMCP(dir, dataDir, definition)
	plugin.Warnings = append(plugin.Warnings, warnings...)
	plugin.MCPServers = make([]string, len(servers))
	for i, server := range servers {
		plugin.MCPServers[i] = strings.TrimPrefix(server.Name, definition.Name+":")
	}
	if _, err := readClaudePlugin(dir, definition.Name); err != nil {
		plugin.Warnings = append(plugin.Warnings, err.Error())
	}
	return plugin, nil
}
