package plugins

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
)

const mcpSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

func MCPServerID(plugin, name string) string {
	return "plugin:" + plugin + ":" + base64.RawURLEncoding.EncodeToString([]byte(name))
}

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

func readMCP(root, path string, portable bool) (map[string]json.RawMessage, error) {
	held, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer held.Close()
	data, err := held.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var document mcpDocument
	if portable {
		err = decodeStrict(data, &document)
		if err == nil && (document.Schema != mcpSchema || document.Servers == nil) {
			err = fmt.Errorf("requires the Agent Plugins MCP schema and mcpServers object")
		}
	} else {
		err = json.Unmarshal(data, &document)
	}
	return document.Servers, err
}

func collectNativeMCP(root string, raw json.RawMessage, entries map[string]json.RawMessage) []string {
	var path string
	var list []json.RawMessage
	var inline map[string]json.RawMessage
	switch {
	case json.Unmarshal(raw, &path) == nil:
		resolved, err := manifest.Resolve(root, path)
		if err != nil {
			return []string{fmt.Sprintf("MCP %s: %v", path, err)}
		}
		relative, err := filepath.Rel(root, resolved)
		if err != nil {
			return []string{fmt.Sprintf("MCP %s: %v", path, err)}
		}
		loaded, err := readMCP(root, relative, false)
		if err != nil {
			return []string{fmt.Sprintf("MCP %s: %v", filepath.Base(path), err)}
		}
		maps.Copy(entries, loaded)
	case json.Unmarshal(raw, &list) == nil && list != nil:
		var warnings []string
		for _, item := range list {
			warnings = append(warnings, collectNativeMCP(root, item, entries)...)
		}
		return warnings
	case json.Unmarshal(raw, &inline) == nil && inline != nil:
		maps.Copy(entries, inline)
	default:
		return []string{"MCP declaration must be a path, server map or array"}
	}
	return nil
}

func loadMCP(root, dataDir string, definition manifest.Manifest) ([]mcpconfig.Server, []string) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, []string{err.Error()}
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, []string{err.Error()}
	}
	portable := definition.ManifestPath == "plugin.json"
	path := ".mcp.json"
	if portable {
		path = "mcp.json"
	}
	loaded, err := readMCP(root, path, portable)
	entries := map[string]json.RawMessage{}
	var warnings []string
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("MCP %s: %v", path, err))
	} else {
		maps.Copy(entries, loaded)
	}
	if !portable && len(definition.MCPServers) != 0 {
		warnings = append(warnings, collectNativeMCP(root, definition.MCPServers, entries)...)
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
		server.ID = MCPServerID(definition.Name, name)
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
