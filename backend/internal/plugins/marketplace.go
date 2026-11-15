package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
)

func (s *Store) marketplace(root string) ([]preparedPlugin, []SkippedPlugin, error) {
	var raw []byte
	for _, path := range []string{".agents/plugins/marketplace.json", ".claude-plugin/marketplace.json"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		raw = data
		break
	}
	if raw == nil {
		prepared, err := s.discover(root)
		return prepared, nil, err
	}
	var document struct {
		Plugins []struct {
			Name   string          `json:"name"`
			Source json.RawMessage `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid marketplace JSON", ErrInvalid)
	}
	if document.Plugins == nil {
		return nil, nil, fmt.Errorf("%w: marketplace needs a plugins array", ErrInvalid)
	}
	var prepared []preparedPlugin
	var skipped []SkippedPlugin
	for _, entry := range document.Plugins {
		if entry.Name == "" {
			return nil, nil, fmt.Errorf("%w: marketplace plugin name is required", ErrInvalid)
		}
		var path string
		if json.Unmarshal(entry.Source, &path) != nil {
			var source struct {
				Kind string `json:"source"`
				Path string `json:"path"`
			}
			if json.Unmarshal(entry.Source, &source) != nil || source.Kind != "local" {
				skipped = append(skipped, SkippedPlugin{Name: entry.Name, Reason: "unsupported marketplace source"})
				continue
			}
			path = source.Path
		}
		resolved, err := manifest.Resolve(root, path)
		if err != nil {
			skipped = append(skipped, SkippedPlugin{Name: entry.Name, Reason: "local source must name a folder inside the repository"})
			continue
		}
		plugin, err := read(resolved, s.dataDir)
		if err != nil || plugin.Name != entry.Name {
			skipped = append(skipped, SkippedPlugin{Name: entry.Name, Reason: "local source has no valid matching plugin manifest"})
			continue
		}
		prepared = append(prepared, preparedPlugin{plugin: plugin, path: resolved})
	}
	if len(prepared) == 0 && len(skipped) == 0 {
		return nil, nil, fmt.Errorf("%w: marketplace contains no plugins", ErrInvalid)
	}
	return prepared, skipped, nil
}
