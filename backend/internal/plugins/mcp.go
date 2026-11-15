package plugins

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
)

const mcpSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

type mcpDocument struct {
	Schema  string                     `json:"$schema"`
	Servers map[string]json.RawMessage `json:"mcpServers"`
}

type stdioConfig struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     string            `json:"cwd"`
}

type remoteConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid trailing JSON")
	}
	return nil
}

func loadMCP(root, dataDir string, definition manifest.Manifest) ([]mcpconfig.Server, []string) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, []string{err.Error()}
	}
	entries := map[string]json.RawMessage{}
	var warnings []string
	load := func(path string, portable bool) {
		held, err := os.OpenRoot(root)
		var data []byte
		if err == nil {
			relative, relErr := filepath.Rel(root, path)
			if relErr == nil {
				data, err = held.ReadFile(relative)
			} else {
				err = relErr
			}
			_ = held.Close()
		}
		if os.IsNotExist(err) {
			return
		}
		if err == nil {
			var document mcpDocument
			if portable {
				err = decodeStrict(data, &document)
				if err == nil && (document.Schema != mcpSchema || document.Servers == nil) {
					err = fmt.Errorf("requires the Agent Plugins MCP schema and mcpServers object")
				}
			} else {
				err = json.Unmarshal(data, &document)
			}
			if err == nil {
				for name, entry := range document.Servers {
					entries[name] = entry
				}
			}
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("MCP %s: %v", filepath.Base(path), err))
		}
	}
	portable := definition.ManifestPath == "plugin.json"
	if portable {
		load(filepath.Join(root, "mcp.json"), true)
	} else {
		load(filepath.Join(root, ".mcp.json"), false)
		var extra func(json.RawMessage)
		extra = func(raw json.RawMessage) {
			var path string
			var list []json.RawMessage
			var inline map[string]json.RawMessage
			switch {
			case json.Unmarshal(raw, &path) == nil:
				resolved, err := manifest.Resolve(root, path)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("MCP %s: %v", path, err))
					return
				}
				load(resolved, false)
			case json.Unmarshal(raw, &list) == nil && list != nil:
				for _, item := range list {
					extra(item)
				}
			case json.Unmarshal(raw, &inline) == nil && inline != nil:
				for name, entry := range inline {
					entries[name] = entry
				}
			default:
				warnings = append(warnings, "MCP declaration must be a path, server map or array")
			}
		}
		if len(definition.MCPServers) != 0 {
			extra(definition.MCPServers)
		}
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)
	var servers []mcpconfig.Server
	for _, name := range names {
		if name == "" {
			warnings = append(warnings, "MCP server name must not be empty")
			continue
		}
		server, err := normalizeMCP(root, filepath.Join(dataDir, definition.Name), entries[name], portable)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("MCP %s: %v", name, err))
			continue
		}
		server.ID = "plugin:" + definition.Name + ":" + base64.RawURLEncoding.EncodeToString([]byte(name))
		server.Name = definition.Name + ":" + name
		server.Source = mcpconfig.SourcePlugin
		server.UpdatedAt = info.ModTime()
		server.Enabled = true
		servers = append(servers, server)
	}
	return servers, warnings
}

func (s *Store) ListMCPServers() ([]mcpconfig.Server, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var servers []mcpconfig.Server
	for _, entry := range entries {
		root := filepath.Join(s.dir, entry.Name())
		definition, err := manifest.Read(root)
		if err != nil || definition.Name != entry.Name() {
			continue
		}
		loaded, _ := loadMCP(root, s.dataDir, definition)
		servers = append(servers, loaded...)
	}
	return servers, nil
}
