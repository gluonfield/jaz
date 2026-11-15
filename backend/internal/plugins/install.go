package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
)

type Source struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref,omitempty"`
	Commit     string `json:"commit"`
}

type preparedPlugin struct {
	plugin   Plugin
	path     string
	source   *Source
	expected *Source
}

func (s *Store) sources() (map[string]Source, error) {
	data, err := os.ReadFile(s.sourcesPath)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Source{}, nil
	}
	if err != nil {
		return nil, err
	}
	var sources map[string]Source
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("plugin source metadata: %w", err)
	}
	if sources == nil {
		sources = map[string]Source{}
	}
	return sources, nil
}

func (s *Store) stageSources(sources map[string]Source) (string, error) {
	data, err := json.Marshal(sources)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(s.sourcesPath), ".plugin-sources-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		_ = os.Remove(path)
	}
	return path, err
}

func (s *Store) publish(prepared []preparedPlugin) (out []Plugin, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.sources()
	if err != nil {
		return nil, err
	}
	next := maps.Clone(current)
	for _, candidate := range prepared {
		if candidate.expected != nil {
			previous, exists := current[candidate.plugin.Name]
			if !exists || previous != *candidate.expected {
				return nil, fmt.Errorf("%w: plugin changed while its update was fetched; retry", ErrInvalid)
			}
		}
		if candidate.source == nil {
			delete(next, candidate.plugin.Name)
		} else {
			next[candidate.plugin.Name] = *candidate.source
		}
	}
	metadata, err := s.stageSources(next)
	if err != nil {
		return nil, err
	}
	defer os.Remove(metadata)
	backups, err := os.MkdirTemp(s.dir, ".previous-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(backups)
	changed := 0
	defer func() {
		if err == nil {
			return
		}
		for i := changed - 1; i >= 0; i-- {
			live := filepath.Join(s.dir, prepared[i].plugin.Name)
			previous := filepath.Join(backups, prepared[i].plugin.Name)
			err = errors.Join(err, os.RemoveAll(live))
			if _, statErr := os.Lstat(previous); statErr == nil {
				err = errors.Join(err, os.Rename(previous, live))
			}
		}
	}()
	out = make([]Plugin, 0, len(prepared))
	for i, candidate := range prepared {
		live := filepath.Join(s.dir, candidate.plugin.Name)
		previous := filepath.Join(backups, candidate.plugin.Name)
		if candidate.path == "" {
			if _, err := manifest.Read(live); errors.Is(err, fs.ErrNotExist) || errors.Is(err, manifest.ErrNotPlugin) {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, candidate.plugin.Name)
			}
		}
		if err = os.Rename(live, previous); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		changed = i + 1
		if candidate.path != "" {
			if err = os.Rename(candidate.path, live); err != nil {
				return nil, err
			}
		}
		candidate.plugin.Source = candidate.source
		out = append(out, candidate.plugin)
	}
	if err = os.Rename(metadata, s.sourcesPath); err != nil {
		return nil, err
	}
	_ = os.RemoveAll(backups)
	return out, nil
}

func (s *Store) discover(root string) ([]preparedPlugin, error) {
	var prepared []preparedPlugin
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		plugin, err := read(path, s.dataDir)
		if errors.Is(err, manifest.ErrNotPlugin) {
			return nil
		}
		if err != nil {
			return err
		}
		prepared = append(prepared, preparedPlugin{plugin: plugin, path: path})
		return fs.SkipDir
	})
	if err == nil && len(prepared) == 0 {
		err = fmt.Errorf("%w: %w", ErrInvalid, manifest.ErrNotPlugin)
	}
	return prepared, err
}

func stagePackages(prepared []preparedPlugin, root string) ([]preparedPlugin, error) {
	seen := map[string]bool{}
	for i := range prepared {
		candidate := &prepared[i]
		if seen[candidate.plugin.Name] {
			return nil, fmt.Errorf("%w: duplicate plugin %s", ErrInvalid, candidate.plugin.Name)
		}
		seen[candidate.plugin.Name] = true
		staged := filepath.Join(root, candidate.plugin.Name)
		if err := os.CopyFS(staged, os.DirFS(candidate.path)); err != nil {
			return nil, err
		}
		candidate.path = staged
	}
	return prepared, nil
}
