package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/skills"
)

type NativePlugin struct {
	Path   string
	Skills []skills.Skill
}

func readClaudePlugin(root, name string) (manifest.Manifest, error) {
	native, err := manifest.ReadClaude(root)
	if err == nil && native.ManifestPath != "" && native.Name != name {
		err = fmt.Errorf("native name %q differs from installed name %q", native.Name, name)
	}
	if err != nil {
		err = fmt.Errorf("Native Claude loading skipped: %w", err)
	}
	return native, err
}

func (s *Store) ClaudePlugins() ([]NativePlugin, error) {
	states := s.loadStates(context.Background())
	return s.ClaudePluginsFiltered(func(id string) bool {
		st, ok := states[id]
		return !ok || st.Enabled
	})
}

func (s *Store) ClaudePluginsFiltered(allow func(string) bool) ([]NativePlugin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dir, _ := s.workspaceDirs()
	directory, err := filepath.Abs(dir)
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
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		root := filepath.Join(directory, entry.Name())
		plugin, err := manifest.Read(root)
		if err != nil || plugin.Name != entry.Name() {
			continue
		}
		if allow != nil && !allow(plugin.Name) {
			continue
		}
		native, err := readClaudePlugin(root, plugin.Name)
		if err != nil {
			continue
		}
		held, _ := skills.LoadPlugin(root, native.Name, native.ExtraSkillPaths)
		out = append(out, NativePlugin{Path: root, Skills: held})
	}
	return out, nil
}
