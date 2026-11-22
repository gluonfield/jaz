package plugins

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	searchResultsMax   = 20
	OAuthCallbackPath  = "/v1/plugins/oauth/callback"
	SearchMCPToolName  = "plugins_search"
	DefaultRefreshWait = 15 * time.Minute
)

type SourceView struct {
	ID            string             `json:"id"`
	Repository    string             `json:"repository"`
	URL           string             `json:"url"`
	Ref           string             `json:"ref,omitempty"`
	Paths         []string           `json:"paths,omitempty"`
	CommitSHA     string             `json:"commit_sha,omitempty"`
	AutoUpdate    bool               `json:"auto_update,omitempty"`
	IsMarketplace bool               `json:"is_marketplace,omitempty"`
	Plugins       []MarketplaceEntry `json:"plugins,omitempty"`
	CheckedAt     time.Time          `json:"checked_at,omitempty"`
}

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

func (s *Service) List(ctx context.Context, loopbackAllowed bool) (CatalogResponse, error) {
	states := s.stateMap(ctx)
	seen := map[string]bool{}
	var out []Plugin

	if s.connections != nil {
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
	record, ok, err := s.state.GetPluginState(ctx, id)
	if err != nil {
		return Plugin{}, err
	}
	if !ok {
		record = storage.PluginStateRecord{PluginID: id}
	}
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
	record, ok, err := s.state.GetPluginState(ctx, id)
	if err != nil {
		return Plugin{}, err
	}
	if !ok {
		record = storage.PluginStateRecord{PluginID: id, Enabled: true}
	}
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
	if s.connections != nil && s.connections.Catalog().IsNative(id) {
		return s.connect.Start(ctx, id, opts)
	}
	plugin, err := s.Get(ctx, id, opts.MCP.LoopbackAllowed)
	if err != nil {
		return connections.ConnectResult{}, err
	}
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
		plugin, err = s.Get(ctx, id, opts.MCP.LoopbackAllowed)
		if err != nil {
			return connections.ConnectResult{}, err
		}
	}
	for _, srv := range plugin.MCPServers {
		if srv.Transport == string(mcpconfig.TransportStreamableHTTP) && srv.URL != "" {
			return s.connect.StartMCP(ctx, plugin.ID, MCPServerID(plugin.ID, srv.Name), plugin.DisplayName, srv.URL, opts.MCP)
		}
	}
	return connections.ConnectResult{}, fmt.Errorf("%w: plugin %q has no remote MCP server to connect", ErrInvalid, id)
}

func (s *Service) DisconnectAccount(ctx context.Context, accountID string) (connections.DisconnectResult, error) {
	if s.connections == nil {
		return connections.DisconnectResult{}, connections.ErrConnectionNotFound
	}
	return s.connections.DisconnectAccount(ctx, accountID)
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
	return s.store.Remove(id)
}

func (s *Service) ListSources(ctx context.Context) ([]SourceView, error) {
	if s.state == nil {
		return []SourceView{}, nil
	}
	records, err := s.state.ListPluginSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SourceView, 0, len(records))
	for _, record := range records {
		var entries []MarketplaceEntry
		_ = json.Unmarshal(record.CatalogJSON, &entries)
		out = append(out, sourceViewFromRecord(record, entries))
	}
	return out, nil
}

func (s *Service) AddSource(ctx context.Context, input RepositoryInput) (SourceView, error) {
	if s.store == nil || s.state == nil {
		return SourceView{}, errors.New("plugin store unavailable")
	}
	record, entries, err := s.store.InspectSource(ctx, input)
	if err != nil {
		return SourceView{}, err
	}
	if err := s.state.SavePluginSource(ctx, record); err != nil {
		return SourceView{}, err
	}
	return sourceViewFromRecord(record, entries), nil
}

func sourceViewFromRecord(record storage.PluginSourceRecord, entries []MarketplaceEntry) SourceView {
	return SourceView{
		ID:            record.ID,
		Repository:    record.Repository,
		URL:           record.URL,
		Ref:           record.Ref,
		Paths:         record.Paths,
		CommitSHA:     record.CommitSHA,
		AutoUpdate:    record.AutoUpdate,
		IsMarketplace: record.IsMarketplace,
		Plugins:       entries,
		CheckedAt:     record.CheckedAt,
	}
}

func (s *Service) RemoveSource(ctx context.Context, id string) error {
	if s.state == nil {
		return ErrNotFound
	}
	deleted, err := s.state.DeletePluginSource(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

// RefreshSources checks configured marketplaces and installed Git plugins via
// deduplicated parallel git ls-remote calls, updating cached catalogs and
// auto-updating sources marked auto_update=true.
func (s *Service) RefreshSources(ctx context.Context) error {
	if s.store == nil || s.state == nil {
		return nil
	}
	sources, err := s.state.ListPluginSources(ctx)
	if err != nil {
		return err
	}
	states, err := s.state.ListPluginStates(ctx)
	if err != nil {
		return err
	}

	type repoRef struct{ url, ref string }
	type remoteResult struct {
		sha string
		err error
	}
	targets := map[repoRef]struct{}{}
	for _, src := range sources {
		targets[repoRef{url: src.URL, ref: src.Ref}] = struct{}{}
	}
	stateURLs := make(map[string]string, len(states))
	for _, st := range states {
		if st.SourceRepository == "" || commitID.MatchString(st.SourceRef) {
			continue
		}
		if repoURL, err := repositoryURL(st.SourceRepository); err == nil {
			stateURLs[st.PluginID] = repoURL
			targets[repoRef{url: repoURL, ref: st.SourceRef}] = struct{}{}
		}
	}

	results := make(map[repoRef]remoteResult, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for target := range targets {
		wg.Add(1)
		go func(t repoRef) {
			defer wg.Done()
			sha, err := RemoteCommit(ctx, t.url, t.ref)
			mu.Lock()
			results[t] = remoteResult{sha: sha, err: err}
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	var errs []error
	autoSources := make(map[string]bool, len(sources))
	for _, src := range sources {
		autoSources[src.ID] = src.AutoUpdate
		res := results[repoRef{url: src.URL, ref: src.Ref}]
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		if res.sha != src.CommitSHA {
			updated, _, inspectErr := s.store.InspectSource(ctx, RepositoryInput{
				Repository: src.Repository,
				Ref:        src.Ref,
				Paths:      src.Paths,
				AutoUpdate: src.AutoUpdate,
			})
			if inspectErr != nil {
				errs = append(errs, inspectErr)
				continue
			}
			src = updated
		} else {
			src.CheckedAt = time.Now().UTC()
		}
		if err := s.state.SavePluginSource(ctx, src); err != nil {
			errs = append(errs, err)
		}
	}
	for _, st := range states {
		repoURL, ok := stateURLs[st.PluginID]
		if !ok {
			continue
		}
		res := results[repoRef{url: repoURL, ref: st.SourceRef}]
		if res.err != nil {
			continue
		}
		if res.sha != st.CommitSHA && autoSources[st.SourceID] {
			if _, updateErr := s.store.Update(ctx, st.PluginID); updateErr != nil {
				errs = append(errs, updateErr)
			}
			continue
		}
		if res.sha != st.RemoteSHA {
			st.RemoteSHA = res.sha
			if saveErr := s.state.SavePluginState(ctx, st); saveErr != nil {
				errs = append(errs, saveErr)
			}
		}
	}
	return errors.Join(errs...)
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
	if s.connections != nil {
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
