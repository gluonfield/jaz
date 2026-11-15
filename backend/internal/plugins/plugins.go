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
	IconDataURL string   `json:"icon_data_url,omitempty"`
	Page        *Page    `json:"page,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func New(layout runtimefiles.Layout) *Store {
	return &Store{dir: layout.Plugins}
}

func (s *Store) List() ([]Plugin, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Plugin{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Plugin{}
	for _, entry := range entries {
		plugin, err := read(filepath.Join(s.dir, entry.Name()))
		if err == nil && plugin.Name == entry.Name() {
			out = append(out, plugin)
		}
	}
	return out, nil
}

// View returns a view's HTML by its reference, "<plugin>/<view>".
func (s *Store) View(ref string) (string, error) {
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
func (s *Store) Install(archive []byte) ([]Plugin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(s.dir, ".install-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := skills.ExtractArchive(archive, work); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var found []Plugin
	var dirs []string
	if err := filepath.WalkDir(work, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		plugin, err := read(path)
		if errors.Is(err, manifest.ErrNotPlugin) {
			return nil
		}
		if err != nil {
			return err
		}
		found = append(found, plugin)
		dirs = append(dirs, path)
		return fs.SkipDir
	}); err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, manifest.ErrNotPlugin)
	}
	for i, plugin := range found {
		dest := filepath.Join(s.dir, plugin.Name)
		if err := os.RemoveAll(dest); err != nil {
			return nil, err
		}
		if err := os.Rename(dirs[i], dest); err != nil {
			return nil, err
		}
	}
	return found, nil
}

func (s *Store) Remove(plugin string) error {
	if !manifest.SafeName(plugin) {
		return fmt.Errorf("%w: name %q", ErrInvalid, plugin)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.dir, plugin)
	if _, err := manifest.Read(dir); errors.Is(err, fs.ErrNotExist) || errors.Is(err, manifest.ErrNotPlugin) {
		return fmt.Errorf("%w: %s", ErrNotFound, plugin)
	}
	return os.RemoveAll(dir)
}

func read(dir string) (Plugin, error) {
	definition, err := manifest.Read(dir)
	if err != nil {
		return Plugin{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	plugin := Plugin{Name: definition.Name, DisplayName: definition.DisplayName, Description: definition.Description, Version: definition.Version, Warnings: definition.Warnings}
	if definition.Page != nil {
		plugin.Page = &Page{View: definition.Page.View, File: definition.Page.File}
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
		if err := validatePage(plugin.Page, plugin.Views); err != nil {
			return Plugin{}, err
		}
	}
	plugin.IconDataURL, err = readIcon(dir, definition.IconPath)
	if err != nil {
		return Plugin{}, err
	}
	return plugin, nil
}
