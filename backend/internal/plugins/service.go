package plugins

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	searchResultsMax       = 20
	OAuthCallbackPath      = "/v1/plugins/oauth/callback"
	SearchMCPToolName      = "plugins_search"
	DefaultRefreshWait     = 15 * time.Minute
	DefaultMarketplaceRepo = "cambridgeadvancedsystems/jaz-plugins-official"
)

type CatalogResponse struct {
	Plugins []Plugin     `json:"plugins"`
	Sources []SourceView `json:"sources"`
}

type Service struct {
	store       *Store
	state       storage.PluginStore
	connections *connections.Service
	connect     *connections.ConnectService
	oauth       *connections.OAuthService
	qr          *connections.QRService
}

func NewService(
	store *Store,
	state storage.PluginStore,
	connService *connections.Service,
	connectService *connections.ConnectService,
	oauth *connections.OAuthService,
	qr *connections.QRService,
) *Service {
	if store != nil {
		store.WithStateStore(state)
		if connService != nil && connService.Catalog() != nil {
			store.WithReservedIDs(connService.Catalog().IsNative)
		}
	}
	return &Service{
		store:       store,
		state:       state,
		connections: connService,
		connect:     connectService,
		oauth:       oauth,
		qr:          qr,
	}
}

func (s *Service) Store() *Store {
	return s.store
}

func (s *Service) OAuth() *connections.OAuthService {
	return s.oauth
}

func (s *Service) QR() *connections.QRService {
	return s.qr
}

func (s *Service) IsEnabled(ctx context.Context, pluginID string) bool {
	return s.store.IsEnabled(ctx, pluginID)
}

func (s *Service) activeWorkspaceID() string {
	if s.store != nil {
		return s.store.ActiveWorkspaceID()
	}
	return storage.LocalWorkspaceID
}

func (s *Service) isLocalWorkspace() bool {
	return s.activeWorkspaceID() == storage.LocalWorkspaceID
}

func (s *Service) List(ctx context.Context, loopbackAllowed bool) (CatalogResponse, error) {
	states := s.stateMap(ctx)
	seen := map[string]bool{}
	var out []Plugin

	if s.isLocalWorkspace() && s.connections != nil {
		natives, err := s.connections.ListPlugins(ctx, loopbackAllowed)
		if err != nil {
			return CatalogResponse{}, err
		}
		for _, native := range natives {
			p := pluginFromNative(native, states[native.ID])
			seen[p.ID] = true
			out = append(out, p)
		}
	}

	if s.store != nil {
		installed, err := s.store.ListWithContext(ctx)
		if err != nil {
			return CatalogResponse{}, err
		}
		for _, p := range installed {
			if seen[p.ID] {
				continue
			}
			p = s.enrichInstalledAuth(ctx, p, loopbackAllowed)
			seen[p.ID] = true
			out = append(out, p)
		}
	}

	sources, err := s.ListSources(ctx)
	if err != nil {
		return CatalogResponse{}, err
	}
	for _, src := range sources {
		for _, entry := range src.Plugins {
			if seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			out = append(out, pluginFromMarketplace(src, entry))
		}
	}

	slices.SortFunc(out, func(a, b Plugin) int {
		return strings.Compare(strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName))
	})
	return CatalogResponse{Plugins: out, Sources: sources}, nil
}

func (s *Service) Get(ctx context.Context, id string, loopbackAllowed bool) (Plugin, error) {
	id = strings.TrimSpace(id)
	catalog, err := s.List(ctx, loopbackAllowed)
	if err != nil {
		return Plugin{}, err
	}
	for _, p := range catalog.Plugins {
		if p.ID == id {
			return p, nil
		}
	}
	return Plugin{}, ErrNotFound
}

func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) (Plugin, error) {
	id = strings.TrimSpace(id)
	if !manifest.SafeName(id) {
		return Plugin{}, fmt.Errorf("%w: invalid plugin id %q", ErrInvalid, id)
	}
	plugin, err := s.Get(ctx, id, true)
	if err != nil {
		return Plugin{}, err
	}
	if !plugin.Installed {
		return Plugin{}, fmt.Errorf("%w: install %s before enabling it", ErrInvalid, id)
	}
	if s.state == nil {
		return Plugin{}, fmt.Errorf("%w: plugin state store unavailable", ErrInvalid)
	}
	ws := s.activeWorkspaceID()
	record, ok, err := s.state.GetPluginState(ctx, ws, id)
	if err != nil {
		return Plugin{}, err
	}
	if !ok {
		record = storage.PluginStateRecord{WorkspaceID: ws, PluginID: id}
	}
	record.WorkspaceID = ws
	record.Enabled = enabled
	if err := s.state.SavePluginState(ctx, record); err != nil {
		return Plugin{}, err
	}
	return s.Get(ctx, id, true)
}

func (s *Service) SaveConfig(ctx context.Context, id string, values map[string]string) (Plugin, error) {
	id = strings.TrimSpace(id)
	if !manifest.SafeName(id) {
		return Plugin{}, fmt.Errorf("%w: invalid plugin id %q", ErrInvalid, id)
	}
	if _, err := s.Get(ctx, id, true); err != nil {
		return Plugin{}, err
	}
	if s.state == nil {
		return Plugin{}, fmt.Errorf("%w: plugin state store unavailable", ErrInvalid)
	}
	ws := s.activeWorkspaceID()
	record, ok, err := s.state.GetPluginState(ctx, ws, id)
	if err != nil {
		return Plugin{}, err
	}
	if !ok {
		record = storage.PluginStateRecord{WorkspaceID: ws, PluginID: id, Enabled: true}
	}
	record.WorkspaceID = ws
	if record.Config == nil {
		record.Config = map[string]string{}
	}
	for key, val := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if val == "" {
			delete(record.Config, key)
		} else {
			record.Config[key] = val
		}
	}
	if err := s.state.SavePluginState(ctx, record); err != nil {
		return Plugin{}, err
	}
	return s.Get(ctx, id, true)
}

func (s *Service) Connect(ctx context.Context, id string, opts connections.StartOptions) (connections.ConnectResult, error) {
	id = strings.TrimSpace(id)
	if s.connect == nil {
		return connections.ConnectResult{}, errors.New("connections are not configured")
	}
	if s.isLocalWorkspace() && s.connections != nil && s.connections.Catalog().IsNative(id) {
		return s.connect.Start(ctx, id, opts)
	}
	plugin, err := s.Get(ctx, id, opts.MCP.LoopbackAllowed)
	if err != nil {
		return connections.ConnectResult{}, err
	}
	installedNow := false
	if !plugin.Installed {
		if plugin.Source == nil || plugin.Source.Repository == "" {
			return connections.ConnectResult{}, fmt.Errorf("%w: plugin %q cannot be connected before installation", ErrInvalid, id)
		}
		var paths []string
		if plugin.Source.Path != "" {
			paths = []string{plugin.Source.Path}
		}
		if _, err := s.store.InstallRepository(ctx, RepositoryInput{
			Repository: plugin.Source.Repository,
			Ref:        plugin.Source.Ref,
			Paths:      paths,
			PluginID:   plugin.ID,
			SourceID:   plugin.Source.ID,
		}); err != nil {
			return connections.ConnectResult{}, err
		}
		installedNow = true
		plugin, err = s.Get(ctx, id, opts.MCP.LoopbackAllowed)
		if err != nil {
			return connections.ConnectResult{}, err
		}
	}
	for _, srv := range plugin.MCPServers {
		if srv.Transport == string(mcpconfig.TransportStreamableHTTP) && srv.URL != "" {
			res, startErr := s.connect.StartMCP(ctx, plugin.ID, MCPServerID(plugin.ID, srv.Name), plugin.DisplayName, srv.URL, opts.MCP)
			if startErr != nil && installedNow && isPureRemoteMCP(plugin) {
				_ = s.store.Remove(plugin.ID)
			}
			return res, startErr
		}
	}
	if installedNow && isPureRemoteMCP(plugin) {
		_ = s.store.Remove(plugin.ID)
	}
	return connections.ConnectResult{}, fmt.Errorf("%w: plugin %q has no remote MCP server to connect", ErrInvalid, id)
}

func (s *Service) DisconnectAccount(ctx context.Context, accountID string) (connections.DisconnectResult, error) {
	if !s.isLocalWorkspace() || s.connections == nil {
		return connections.DisconnectResult{}, connections.ErrConnectionNotFound
	}
	result, err := s.connections.DisconnectAccount(ctx, accountID)
	if err != nil {
		return connections.DisconnectResult{}, err
	}
	if result.Provider != "" {
		if clearErr := s.clearPluginMCPTokens(ctx, result.Provider); clearErr != nil {
			return connections.DisconnectResult{}, clearErr
		}
		if err := s.removeIfRemoteMCPOnly(ctx, result.Provider); err != nil {
			return connections.DisconnectResult{}, err
		}
	}
	return result, nil
}

func isPureRemoteMCP(plugin Plugin) bool {
	if len(plugin.Skills) > 0 || len(plugin.Views) > 0 || len(plugin.MCPServers) == 0 {
		return false
	}
	for _, srv := range plugin.MCPServers {
		if srv.Transport != string(mcpconfig.TransportStreamableHTTP) {
			return false
		}
	}
	return true
}

func (s *Service) removeIfRemoteMCPOnly(ctx context.Context, pluginID string) error {
	if s.store == nil || (s.connections != nil && s.connections.Catalog() != nil && s.connections.Catalog().IsNative(pluginID)) {
		return nil
	}
	plugin, err := s.store.Read(ctx, pluginID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if !isPureRemoteMCP(plugin) {
		return nil
	}
	if s.connections != nil {
		accounts, err := s.connections.ListAccounts(ctx, pluginID)
		if err != nil {
			return err
		}
		if len(accounts) > 0 {
			return nil
		}
	}
	if err := s.store.Remove(pluginID); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func (s *Service) Install(ctx context.Context, input RepositoryInput) (InstallResult, error) {
	if s.store == nil {
		return InstallResult{}, errors.New("plugin store unavailable")
	}
	if strings.TrimSpace(input.Repository) == "" && strings.TrimSpace(input.PluginID) != "" {
		return s.installFromBrowse(ctx, strings.TrimSpace(input.PluginID))
	}
	return s.store.InstallRepository(ctx, input)
}

func (s *Service) installFromBrowse(ctx context.Context, pluginID string) (InstallResult, error) {
	sources, err := s.ListSources(ctx)
	if err != nil {
		return InstallResult{}, err
	}
	for _, src := range sources {
		for _, entry := range src.Plugins {
			if entry.ID != pluginID {
				continue
			}
			repo := cmp.Or(entry.Repository, src.Repository)
			ref := cmp.Or(entry.Ref, src.Ref)
			var paths []string
			if entry.Path != "" {
				paths = []string{entry.Path}
			}
			return s.store.InstallRepository(ctx, RepositoryInput{
				Repository: repo,
				Ref:        ref,
				Paths:      paths,
				PluginID:   entry.ID,
				SourceID:   src.ID,
				AutoUpdate: src.AutoUpdate,
			})
		}
	}
	return InstallResult{}, ErrNotFound
}

func (s *Service) Update(ctx context.Context, id string) (InstallResult, error) {
	if s.store == nil {
		return InstallResult{}, errors.New("plugin store unavailable")
	}
	return s.store.Update(ctx, id)
}

func (s *Service) Remove(ctx context.Context, id string) error {
	if s.store == nil {
		return errors.New("plugin store unavailable")
	}
	id = strings.TrimSpace(id)
	serverIDs := s.pluginMCPServerIDs(ctx, id)
	if err := s.store.Remove(id); err != nil {
		return err
	}
	if !s.isLocalWorkspace() || s.connections == nil {
		return nil
	}
	var errs []error
	accounts, err := s.connections.ListAccounts(ctx, id)
	if err != nil {
		errs = append(errs, err)
	}
	for _, account := range accounts {
		if _, discErr := s.connections.DisconnectAccount(ctx, account.ID); discErr != nil && !errors.Is(discErr, connections.ErrConnectionNotFound) {
			errs = append(errs, discErr)
		}
	}
	if clearErr := s.connections.ClearMCPTokens(ctx, serverIDs...); clearErr != nil {
		errs = append(errs, clearErr)
	}
	return errors.Join(errs...)
}

func (s *Service) pluginMCPServerIDs(ctx context.Context, pluginID string) []string {
	if s.store == nil || pluginID == "" {
		return nil
	}
	plugin, err := s.store.Read(ctx, pluginID)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(plugin.MCPServers))
	for _, srv := range plugin.MCPServers {
		ids = append(ids, MCPServerID(plugin.ID, srv.Name))
	}
	return ids
}

func (s *Service) clearPluginMCPTokens(ctx context.Context, pluginID string) error {
	if s.connections == nil {
		return nil
	}
	return s.connections.ClearMCPTokens(ctx, s.pluginMCPServerIDs(ctx, pluginID)...)
}

func (s *Service) ListMCPServers() ([]mcpconfig.Server, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.ListMCPServers()
}

func (s *Service) ClaudePlugins() ([]NativePlugin, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.ClaudePlugins()
}

func (s *Service) stateMap(ctx context.Context) map[string]storage.PluginStateRecord {
	if s == nil || s.store == nil {
		return map[string]storage.PluginStateRecord{}
	}
	return s.store.loadStates(ctx)
}

func (s *Service) enrichInstalledAuth(ctx context.Context, plugin Plugin, loopbackAllowed bool) Plugin {
	var hasRemoteHTTP bool
	for _, srv := range plugin.MCPServers {
		if srv.Transport == string(mcpconfig.TransportStreamableHTTP) && srv.URL != "" {
			hasRemoteHTTP = true
			break
		}
	}
	if !hasRemoteHTTP {
		return plugin
	}
	auth := &PluginAuth{
		Kind:   "mcp_oauth",
		Status: "available",
	}
	if s.isLocalWorkspace() && s.connections != nil {
		if accounts, err := s.connections.ListAccounts(ctx, plugin.ID); err == nil && len(accounts) > 0 {
			auth.Connected = true
			auth.Accounts = accounts
		}
		if client := s.connections.Catalog().OAuthClientByID(plugin.ID); client.UsesLoopback() && !loopbackAllowed {
			auth.Status = "local_sign_in_required"
			auth.Reason = fmt.Sprintf("Sign in to %s from the device running the local Jaz backend.", plugin.DisplayName)
		}
	}
	plugin.Auth = auth
	return plugin
}

func pluginFromNative(native integrations.Plugin, state storage.PluginStateRecord) Plugin {
	enabled := true
	if state.PluginID != "" {
		enabled = state.Enabled
	}
	caps := make([]string, len(native.Capabilities))
	for i, c := range native.Capabilities {
		caps[i] = string(c)
	}
	tools := make([]PluginTool, len(native.Tools))
	for i, t := range native.Tools {
		tools[i] = PluginTool{Name: t.Name, Description: t.Description, Risk: string(t.Risk)}
	}
	authKind := string(native.PrimaryAuthKind())
	connected := native.Connection != nil && native.Connection.Status == integrations.PluginConnectionStatusConnected
	var accounts []integrations.Connection
	if native.Connection != nil {
		accounts = native.Connection.Accounts
	}
	execution := "cloud"
	if native.PrimaryAuthKind() == integrations.AuthKindSession {
		execution = "device"
	}
	return Plugin{
		ID:               native.ID,
		Name:             native.ID,
		DisplayName:      native.Name,
		Description:      native.Description,
		Category:         native.Category,
		Author:           "Jaz",
		SuggestedPrompts: native.Examples,
		Icon: PluginIcon{
			Kind:       string(native.Icon.Kind),
			Value:      native.Icon.Value,
			Background: native.Icon.Background,
		},
		Origin:       "builtin",
		Execution:    execution,
		Installed:    true,
		Enabled:      enabled,
		Capabilities: caps,
		Tools:        tools,
		Auth: &PluginAuth{
			Kind:         authKind,
			Status:       native.Implementation.Status,
			Reason:       native.Implementation.Reason,
			MultiAccount: native.MultiAccount,
			Connected:    connected,
			Accounts:     accounts,
		},
	}
}

func pluginFromMarketplace(src SourceView, entry MarketplaceEntry) Plugin {
	displayName := cmp.Or(entry.DisplayName, entry.ID)
	icon := PluginIcon{Kind: "initials", Value: initialsFor(displayName)}
	if entry.IconDataURL != "" {
		kind := "data"
		if strings.HasPrefix(entry.IconDataURL, "https://") {
			kind = "url"
		}
		icon = PluginIcon{Kind: kind, Value: entry.IconDataURL}
	}
	var auth *PluginAuth
	if entry.HasRemoteMCP {
		auth = &PluginAuth{Kind: "mcp_oauth", Status: "available"}
	}
	return Plugin{
		ID:               entry.ID,
		Name:             entry.ID,
		DisplayName:      displayName,
		Description:      entry.Description,
		Version:          entry.Version,
		Category:         entry.Category,
		Author:           entry.Author,
		Homepage:         entry.Homepage,
		Repository:       cmp.Or(entry.Repository, src.Repository),
		SuggestedPrompts: entry.SuggestedPrompts,
		Icon:             icon,
		IconDataURL:      entry.IconDataURL,
		Origin:           "marketplace",
		Execution:        cmp.Or(entry.Execution, "cloud"),
		Installed:        false,
		Enabled:          false,
		Auth:             auth,
		Source: &Source{
			ID:         src.ID,
			Repository: cmp.Or(entry.Repository, src.Repository),
			Ref:        cmp.Or(entry.Ref, src.Ref),
			Path:       entry.Path,
			Commit:     src.CommitSHA,
			AutoUpdate: src.AutoUpdate,
		},
	}
}
