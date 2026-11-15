// Package plugins keeps installed plugins. A plugin is a folder with a
// plugin.json, skills under skills/ and HTML views under views/.
package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/skills"
)

const manifestFile = "plugin.json"

var (
	ErrNotFound = errors.New("plugin not found")
	ErrInvalid  = errors.New("invalid plugin")
	name        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

type Plugin struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Skills      []string `json:"skills"`
	Views       []string `json:"views"`
	Page        *Page    `json:"page,omitempty"`
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
	if !ok || !name.MatchString(plugin) || !name.MatchString(view) {
		return "", fmt.Errorf("%w: view %q is not <plugin>/<view>", ErrInvalid, ref)
	}
	html, err := os.ReadFile(filepath.Join(s.dir, plugin, "views", view+".html"))
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
		if _, err := os.Stat(filepath.Join(path, manifestFile)); err != nil {
			return nil
		}
		plugin, err := read(path)
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
		return nil, fmt.Errorf("%w: the archive holds no %s", ErrInvalid, manifestFile)
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
	if !name.MatchString(plugin) {
		return fmt.Errorf("%w: name %q", ErrInvalid, plugin)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.dir, plugin)
	if _, err := os.Stat(filepath.Join(dir, manifestFile)); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, plugin)
	}
	return os.RemoveAll(dir)
}

func read(dir string) (Plugin, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return Plugin{}, err
	}
	var plugin Plugin
	if err := json.Unmarshal(data, &plugin); err != nil {
		return Plugin{}, fmt.Errorf("%w: %s: %v", ErrInvalid, manifestFile, err)
	}
	if !name.MatchString(plugin.Name) {
		return Plugin{}, fmt.Errorf("%w: name %q must be lower-case letters, digits and hyphens", ErrInvalid, plugin.Name)
	}
	held, err := skills.LoadDir(filepath.Join(dir, "skills"))
	if err != nil {
		return Plugin{}, err
	}
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
		if err := plugin.Page.validate(plugin.Views); err != nil {
			return Plugin{}, err
		}
		plugin.Page.Icon, err = readIcon(dir, plugin.Page.Icon)
		if err != nil {
			return Plugin{}, err
		}
	}
	return plugin, nil
}
