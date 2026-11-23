// Package plugins owns Jaz's unified plugin catalog, Git/marketplace installer,
// enablement state, views, and runtime projection for skills and MCP servers.
package plugins

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/skills"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/pkg/integrations"
)

var (
	ErrNotFound = errors.New("plugin not found")
	ErrInvalid  = errors.New("invalid plugin")
	ErrConflict = errors.New("plugin conflict")
	name        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

type Page struct {
	View string `json:"view"`
}

type PluginIcon struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Background string `json:"background,omitempty"`
}

type PluginSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type PluginMCPServer struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	URL       string `json:"-"`
}

type PluginTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Risk        string `json:"risk,omitempty"`
}

type PluginAuth struct {
	Kind         string                    `json:"kind"`
	Status       string                    `json:"status"`
	Reason       string                    `json:"reason,omitempty"`
	MultiAccount bool                      `json:"multi_account,omitempty"`
	Connected    bool                      `json:"connected"`
	Accounts     []integrations.Connection `json:"accounts,omitempty"`
}

type Source struct {
	ID              string `json:"id,omitempty"`
	Repository      string `json:"repository"`
	Ref             string `json:"ref,omitempty"`
	Path            string `json:"path,omitempty"`
	Commit          string `json:"commit"`
	RemoteCommit    string `json:"remote_commit,omitempty"`
	UpdateAvailable bool   `json:"update_available,omitempty"`
	AutoUpdate      bool   `json:"auto_update,omitempty"`
}

type Plugin struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	DisplayName      string            `json:"display_name"`
	Description      string            `json:"description,omitempty"`
	Version          string            `json:"version,omitempty"`
	Category         string            `json:"category,omitempty"`
	Author           string            `json:"author,omitempty"`
	AuthorURL        string            `json:"author_url,omitempty"`
	Homepage         string            `json:"homepage,omitempty"`
	Repository       string            `json:"repository,omitempty"`
	Keywords         []string          `json:"keywords,omitempty"`
	SuggestedPrompts []string          `json:"suggested_prompts,omitempty"`
	Icon             PluginIcon        `json:"icon"`
	IconDataURL      string            `json:"icon_data_url,omitempty"`
	Origin           string            `json:"origin"`
	Execution        string            `json:"execution,omitempty"`
	Installed        bool              `json:"installed"`
	Enabled          bool              `json:"enabled"`
	Capabilities     []string          `json:"capabilities,omitempty"`
	Skills           []PluginSkill     `json:"skills,omitempty"`
	Views            []string          `json:"views,omitempty"`
	MCPServers       []PluginMCPServer `json:"mcp_servers,omitempty"`
	Tools            []PluginTool      `json:"tools,omitempty"`
	ConfigVariables  []ConfigVariable  `json:"config_variables,omitempty"`
	ConfigSet        map[string]bool   `json:"config_set,omitempty"`
	Auth             *PluginAuth       `json:"auth,omitempty"`
	Page             *Page             `json:"page,omitempty"`
	Source           *Source           `json:"source,omitempty"`
	Warnings         []string          `json:"warnings,omitempty"`
}

type Store struct {
	dir          string
	dataDir      string
	stateStore   storage.PluginStore
	reservedIDs  func(string) bool
	oauthClients func(string) mcpconfig.OAuthConfig
	mu           sync.RWMutex
}

func New(layout runtimefiles.Layout) *Store {
	return &Store{
		dir:     layout.Plugins,
		dataDir: filepath.Join(layout.Root, "plugin-data"),
	}
}

func NewStore(layout runtimefiles.Layout, state storage.PluginStore, catalog *connections.Catalog) *Store {
	store := New(layout).WithStateStore(state)
	if catalog != nil {
		store.reservedIDs = catalog.IsNative
		store.oauthClients = catalog.OAuthClientByID
	}
	return store
}

func (s *Store) WithStateStore(store storage.PluginStore) *Store {
	s.stateStore = store
	return s
}

func (s *Store) WithReservedIDs(fn func(string) bool) *Store {
	s.reservedIDs = fn
	return s
}

func (s *Store) IsEnabled(ctx context.Context, pluginID string) bool {
	if s == nil || s.stateStore == nil {
		return true
	}
	record, ok, err := s.stateStore.GetPluginState(ctx, pluginID)
	if err != nil || !ok {
		return true
	}
	return record.Enabled
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) List() ([]Plugin, error) {
	return s.ListWithContext(context.Background())
}

func (s *Store) ListWithContext(ctx context.Context) ([]Plugin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Plugin{}, nil
	}
	if err != nil {
		return nil, err
	}
	states := s.loadStates(ctx)
	out := make([]Plugin, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		state := states[entry.Name()]
		plugin, err := readPackage(filepath.Join(s.dir, entry.Name()), s.dataDir, state.Config)
		if err != nil || plugin.ID != entry.Name() {
			continue
		}
		plugin = applyState(plugin, state)
		out = append(out, plugin)
	}
	slices.SortFunc(out, func(a, b Plugin) int {
		return strings.Compare(strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName))
	})
	return out, nil
}

func (s *Store) Read(ctx context.Context, id string) (Plugin, error) {
	if !manifest.SafeName(id) {
		return Plugin{}, fmt.Errorf("%w: invalid plugin id %q", ErrInvalid, id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	states := s.loadStates(ctx)
	state := states[id]
	plugin, err := readPackage(filepath.Join(s.dir, id), s.dataDir, state.Config)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, manifest.ErrNotPlugin) {
		return Plugin{}, ErrNotFound
	}
	if err != nil {
		return Plugin{}, err
	}
	if plugin.ID != id {
		return Plugin{}, ErrNotFound
	}
	return applyState(plugin, state), nil
}

func (s *Store) View(ref string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.view(ref)
}

func (s *Store) view(ref string) (string, error) {
	plugin, view, ok := strings.Cut(ref, "/")
	if !ok || !manifest.SafeName(plugin) || !name.MatchString(view) {
		return "", fmt.Errorf("%w: view %q is not <plugin>/<view>", ErrInvalid, ref)
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return "", viewLookupError(ref, err)
	}
	defer root.Close()
	installed, err := root.OpenRoot(plugin)
	if err != nil {
		return "", viewLookupError(ref, err)
	}
	defer installed.Close()
	html, err := installed.ReadFile("views/" + view + ".html")
	return string(html), viewLookupError(ref, err)
}

func viewLookupError(ref string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: view %q", ErrNotFound, ref)
	}
	return err
}

func (s *Store) Remove(plugin string) error {
	if !manifest.SafeName(plugin) {
		return fmt.Errorf("%w: name %q", ErrInvalid, plugin)
	}
	if s.reservedIDs != nil && s.reservedIDs(plugin) {
		return fmt.Errorf("%w: %s is a built-in integration and cannot be removed", ErrInvalid, plugin)
	}
	_, err := s.publish(context.Background(), []preparedPlugin{{plugin: Plugin{ID: plugin, Name: plugin}}})
	return err
}

func (s *Store) loadStates(ctx context.Context) map[string]storage.PluginStateRecord {
	out := map[string]storage.PluginStateRecord{}
	if s.stateStore == nil {
		return out
	}
	records, err := s.stateStore.ListPluginStates(ctx)
	if err != nil {
		return out
	}
	for _, record := range records {
		out[record.PluginID] = record
	}
	return out
}

func applyState(plugin Plugin, state storage.PluginStateRecord) Plugin {
	plugin.Installed = true
	if state.PluginID == "" {
		plugin.Enabled = true
	} else {
		plugin.Enabled = state.Enabled
	}
	if len(state.Config) > 0 {
		plugin.ConfigSet = make(map[string]bool, len(state.Config))
		for key, value := range state.Config {
			if value != "" {
				plugin.ConfigSet[key] = true
			}
		}
	}
	if state.SourceRepository != "" {
		plugin.Source = &Source{
			ID:              state.SourceID,
			Repository:      state.SourceRepository,
			Ref:             state.SourceRef,
			Path:            state.SourcePath,
			Commit:          state.CommitSHA,
			RemoteCommit:    state.RemoteSHA,
			UpdateAvailable: state.RemoteSHA != "" && state.CommitSHA != "" && state.RemoteSHA != state.CommitSHA,
		}
	}
	return plugin
}

func initialsFor(label string) string {
	runes := []rune(strings.TrimSpace(label))
	if len(runes) > 2 {
		runes = runes[:2]
	}
	return strings.ToUpper(string(runes))
}

func readPackage(dir, dataDir string, configValues map[string]string) (Plugin, error) {
	definition, err := manifest.Read(dir)
	if err != nil {
		if !errors.Is(err, manifest.ErrNotPlugin) {
			return Plugin{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		fallback, ok := readBareSkillsManifest(dir)
		if !ok {
			return Plugin{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		definition = fallback
	}
	plugin := Plugin{
		ID:               definition.Name,
		Name:             definition.Name,
		DisplayName:      definition.DisplayName,
		Description:      definition.Description,
		Version:          definition.Version,
		Category:         definition.Category,
		Author:           definition.Author,
		AuthorURL:        definition.AuthorURL,
		Homepage:         definition.Homepage,
		Repository:       definition.Repository,
		Keywords:         definition.Keywords,
		SuggestedPrompts: definition.SuggestedPrompts,
		Origin:           "git",
		Installed:        true,
		Enabled:          true,
		Warnings:         definition.Warnings,
	}
	if definition.Page != nil {
		plugin.Page = &Page{View: definition.Page.View}
	}
	held, warnings := skills.LoadPlugin(dir, definition.Name, definition.ExtraSkillPaths)
	plugin.Warnings = append(plugin.Warnings, warnings...)
	plugin.Skills = make([]PluginSkill, len(held))
	for i, skill := range held {
		plugin.Skills[i] = PluginSkill{Name: skill.Name, Description: skill.Description}
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
		if !name.MatchString(plugin.Page.View) || !slices.Contains(plugin.Views, plugin.Page.View) {
			return Plugin{}, fmt.Errorf("%w: page view does not exist", ErrInvalid)
		}
	}
	iconDataURL, err := readIcon(dir, definition.IconPath)
	if err != nil {
		return Plugin{}, err
	}
	if iconDataURL != "" {
		plugin.IconDataURL = iconDataURL
		if strings.HasPrefix(iconDataURL, "https://") {
			plugin.Icon = PluginIcon{Kind: "url", Value: iconDataURL}
		} else {
			plugin.Icon = PluginIcon{Kind: "data", Value: iconDataURL}
		}
	}
	if plugin.Icon.Kind == "" {
		plugin.Icon = PluginIcon{Kind: "initials", Value: initialsFor(plugin.DisplayName)}
	}
	servers, variables, mcpWarnings := loadMCP(dir, dataDir, definition, configValues)
	plugin.Warnings = append(plugin.Warnings, mcpWarnings...)
	plugin.ConfigVariables = variables
	plugin.MCPServers = make([]PluginMCPServer, len(servers))
	for i, server := range servers {
		plugin.MCPServers[i] = PluginMCPServer{
			Name:      strings.TrimPrefix(server.Name, definition.Name+":"),
			Transport: string(server.Transport),
			URL:       server.URL,
		}
	}
	var capabilities []string
	if len(plugin.Skills) > 0 {
		capabilities = append(capabilities, "skills")
	}
	if len(plugin.MCPServers) > 0 {
		capabilities = append(capabilities, "mcp")
	}
	if len(plugin.Views) > 0 || plugin.Page != nil {
		capabilities = append(capabilities, "views")
	}
	plugin.Capabilities = capabilities
	if _, err := readClaudePlugin(dir, definition.Name); err != nil {
		plugin.Warnings = append(plugin.Warnings, err.Error())
	}
	return plugin, nil
}

func readBareSkillsManifest(dir string) (manifest.Manifest, bool) {
	base := filepath.Base(dir)
	if !manifest.SafeName(base) {
		return manifest.Manifest{}, false
	}
	held, _ := skills.LoadPlugin(dir, base, nil)
	if len(held) == 0 {
		return manifest.Manifest{}, false
	}
	return manifest.Manifest{
		Name:        base,
		DisplayName: base,
	}, true
}
