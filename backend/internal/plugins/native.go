package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/skills"
)

type NativePlugin struct {
	Path   string
	Skills []skills.Skill
}

func (s *Store) ClaudePlugins() ([]NativePlugin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	directory, err := filepath.Abs(s.dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []NativePlugin
	for _, entry := range entries {
		root := filepath.Join(directory, entry.Name())
		plugin, err := manifest.Read(root)
		if err != nil || plugin.Name != entry.Name() {
			continue
		}
		native, err := manifest.ReadClaude(root)
		if err != nil {
			return nil, fmt.Errorf("Claude plugin %s: %w", plugin.Name, err)
		}
		if native.Name != plugin.Name {
			return nil, fmt.Errorf("%w: Claude plugin name %q differs from installed name %q", ErrInvalid, native.Name, plugin.Name)
		}
		held, _ := skills.LoadPlugin(root, native.Name, native.ExtraSkillPaths)
		out = append(out, NativePlugin{Path: root, Skills: held})
	}
	return out, nil
}
