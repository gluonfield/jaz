package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

func Read(dir string) (Manifest, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Manifest{}, err
	}
	defer root.Close()
	for _, path := range []string{"plugin.json", ".claude-plugin/plugin.json", ".codex-plugin/plugin.json"} {
		data, err := root.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Manifest{}, err
		}
		var plugin Manifest
		if path == "plugin.json" {
			plugin, err = readCore(data)
		} else {
			plugin, err = readNative(data, path)
		}
		if err != nil {
			return Manifest{}, fmt.Errorf("%s: %w", path, err)
		}
		if !SafeName(plugin.Name) {
			return Manifest{}, fmt.Errorf("%s: invalid plugin name", path)
		}
		if plugin.DisplayName == "" {
			plugin.DisplayName = plugin.Name
		}
		plugin.ManifestPath = path
		return plugin, nil
	}
	return Manifest{}, ErrNotPlugin
}

type paths []string

func (p *paths) UnmarshalJSON(data []byte) error {
	var single string
	if json.Unmarshal(data, &single) == nil && single != "" {
		*p = []string{single}
		return nil
	}
	var multiple []string
	if err := json.Unmarshal(data, &multiple); err != nil || multiple == nil {
		return errors.New("skills must be a path or an array of paths")
	}
	*p = multiple
	return nil
}

type claudeManifest struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName"`
	Description string          `json:"description"`
	Version     string          `json:"version"`
	Skills      paths           `json:"skills"`
	MCPServers  json.RawMessage `json:"mcpServers"`
}

type codexManifest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     string          `json:"version"`
	Skills      paths           `json:"skills"`
	MCPServers  json.RawMessage `json:"mcpServers"`
	Interface   struct {
		DisplayName string `json:"displayName"`
	} `json:"interface"`
}

func readNative(data []byte, path string) (Manifest, error) {
	if path == ".claude-plugin/plugin.json" {
		var input claudeManifest
		if err := json.Unmarshal(data, &input); err != nil {
			return Manifest{}, err
		}
		return Manifest{Name: input.Name, DisplayName: input.DisplayName, Description: input.Description, Version: input.Version, ExtraSkillPaths: input.Skills, MCPServers: input.MCPServers}, nil
	}
	var input codexManifest
	if err := json.Unmarshal(data, &input); err != nil {
		return Manifest{}, err
	}
	return Manifest{Name: input.Name, DisplayName: input.Interface.DisplayName, Description: input.Description, Version: input.Version, ExtraSkillPaths: input.Skills, MCPServers: input.MCPServers}, nil
}
