package plugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
)

func MCPServerID(plugin, name string) string {
	return "plugin:" + plugin + ":" + base64.RawURLEncoding.EncodeToString([]byte(name))
}

type ConfigVariable struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
}

type mcpDocument struct {
	Servers   map[string]json.RawMessage `json:"mcpServers"`
	Variables []ConfigVariable           `json:"variables"`
}

type stdioConfig struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     string            `json:"cwd"`
}

type remoteConfig struct {
	Type      string            `json:"type"`
	URL       string            `json:"url"`
	ServerURL string            `json:"serverUrl"`
	Headers   map[string]string `json:"headers"`
}

func readMCPFile(root, path string) (mcpDocument, error) {
	held, err := os.OpenRoot(root)
	if err != nil {
		return mcpDocument{}, err
	}
	defer held.Close()
	data, err := held.ReadFile(path)
	if os.IsNotExist(err) {
		return mcpDocument{}, nil
	}
	if err != nil {
		return mcpDocument{}, err
	}
	var document mcpDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return mcpDocument{}, err
	}
	return document, nil
}

func collectManifestMCP(root string, raw json.RawMessage, entries map[string]json.RawMessage, variables *[]ConfigVariable) []string {
	var path string
	var list []json.RawMessage
	var inline map[string]json.RawMessage
	switch {
	case json.Unmarshal(raw, &path) == nil && path != "":
		resolved, err := manifest.Resolve(root, path)
		if err != nil {
			return []string{fmt.Sprintf("MCP %s: %v", path, err)}
		}
		relative, err := filepath.Rel(root, resolved)
		if err != nil {
			return []string{fmt.Sprintf("MCP %s: %v", path, err)}
		}
		doc, err := readMCPFile(root, filepath.ToSlash(relative))
		if err != nil {
			return []string{fmt.Sprintf("MCP %s: %v", filepath.Base(path), err)}
		}
		maps.Copy(entries, doc.Servers)
		*variables = mergeVariables(*variables, doc.Variables)
	case json.Unmarshal(raw, &list) == nil && list != nil:
		var warnings []string
		for _, item := range list {
			warnings = append(warnings, collectManifestMCP(root, item, entries, variables)...)
		}
		return warnings
	case json.Unmarshal(raw, &inline) == nil && inline != nil:
		maps.Copy(entries, inline)
	default:
		return []string{"MCP declaration must be a path, server map or array"}
	}
	return nil
}

func loadMCP(root, dataDir string, definition manifest.Manifest, configValues map[string]string) ([]mcpconfig.Server, []ConfigVariable, []string) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, []string{err.Error()}
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, nil, []string{err.Error()}
	}
	entries := map[string]json.RawMessage{}
	var variables []ConfigVariable
	var warnings []string
	for _, candidate := range []string{"mcp.json", ".mcp.json", "mcp_config.json"} {
		doc, err := readMCPFile(root, candidate)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("MCP %s: %v", candidate, err))
			continue
		}
		maps.Copy(entries, doc.Servers)
		variables = mergeVariables(variables, doc.Variables)
	}
	if len(definition.MCPServers) != 0 {
		warnings = append(warnings, collectManifestMCP(root, definition.MCPServers, entries, &variables)...)
	}
	names := make([]string, 0, len(entries))
	for serverName := range entries {
		names = append(names, serverName)
	}
	slices.Sort(names)
	var servers []mcpconfig.Server
	for _, serverName := range names {
		if serverName == "" {
			warnings = append(warnings, "MCP server name must not be empty")
			continue
		}
		var entryVars struct {
			Variables []ConfigVariable `json:"variables"`
		}
		if json.Unmarshal(entries[serverName], &entryVars) == nil && len(entryVars.Variables) > 0 {
			variables = mergeVariables(variables, entryVars.Variables)
		}
		server, err := normalizeMCP(root, filepath.Join(dataDir, definition.Name), entries[serverName], configValues)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("MCP %s: %v", serverName, err))
			continue
		}
		server.ID = MCPServerID(definition.Name, serverName)
		server.Name = definition.Name + ":" + serverName
		server.Source = mcpconfig.SourcePlugin
		server.UpdatedAt = info.ModTime()
		server.Enabled = true
		servers = append(servers, server)
	}
	return servers, variables, warnings
}

func mergeVariables(base, extra []ConfigVariable) []ConfigVariable {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]int, len(base))
	for i, v := range base {
		seen[v.Name] = i
	}
	for _, v := range extra {
		if v.Name == "" {
			continue
		}
		if idx, ok := seen[v.Name]; ok {
			base[idx] = v
			continue
		}
		seen[v.Name] = len(base)
		base = append(base, v)
	}
	return base
}

func (s *Store) ListMCPServers() ([]mcpconfig.Server, error) {
	states := s.loadStates(context.Background())
	servers, err := s.ListMCPServersWithConfig(
		func(id string) bool {
			st, ok := states[id]
			return !ok || st.Enabled
		},
		func(id string) map[string]string {
			return states[id].Config
		},
	)
	if err != nil {
		return nil, err
	}
	if s.oauthClients != nil {
		for i := range servers {
			pluginID, _, _ := strings.Cut(servers[i].Name, ":")
			if client := s.oauthClients(pluginID); client.ClientID != "" || client.ClientSecretEnvVar != "" {
				servers[i].OAuth = client
			}
		}
	}
	return servers, nil
}

func (s *Store) ListMCPServersWithConfig(enabled func(string) bool, configFor func(string) map[string]string) ([]mcpconfig.Server, error) {
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
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(s.dir, entry.Name())
		definition, err := manifest.Read(root)
		if err != nil || definition.Name != entry.Name() {
			continue
		}
		if enabled != nil && !enabled(definition.Name) {
			continue
		}
		var values map[string]string
		if configFor != nil {
			values = configFor(definition.Name)
		}
		loaded, _, _ := loadMCP(root, s.dataDir, definition, values)
		servers = append(servers, loaded...)
	}
	return servers, nil
}
