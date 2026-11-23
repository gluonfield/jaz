package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/coordinator"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	defaultSyncInterval = 30 * time.Second
	cacheTTL            = 30 * time.Second
)

type cachedEndpoint struct {
	raw json.RawMessage
	at  time.Time
}

type Service struct {
	store  storage.CloudMirrorStore
	client *cloudClient
	log    *log.Logger

	mu                sync.RWMutex
	accounts          map[string]storage.CloudAccount
	activeWorkspaceID string
	accountGen        uint64
	activeSyncCancel  context.CancelFunc
	endpointCache     map[string]cachedEndpoint
	onAccountChanged  []func(context.Context)

	syncMu sync.Mutex
	wake   chan struct{}
}

func New(store storage.CloudMirrorStore, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}
	s := &Service{
		store:             store,
		client:            newCloudClient(),
		log:               logger.WithPrefix("cloudsync"),
		accounts:          map[string]storage.CloudAccount{},
		activeWorkspaceID: LocalWorkspaceID,
		endpointCache:     map[string]cachedEndpoint{},
		wake:              make(chan struct{}, 1),
	}
	ctx := context.Background()
	if accounts, err := store.ListCloudAccounts(ctx); err == nil {
		for _, acc := range accounts {
			if acc.ID != "" {
				s.accounts[acc.ID] = acc
			}
		}
	} else {
		s.log.Warn("load cloud accounts failed", "error", err)
	}
	if activeID, err := store.LoadActiveWorkspaceID(ctx); err == nil {
		if activeID == LocalWorkspaceID {
			s.activeWorkspaceID = LocalWorkspaceID
		} else if _, ok := s.accounts[activeID]; ok {
			s.activeWorkspaceID = activeID
		}
	} else {
		s.log.Warn("load active workspace failed", "error", err)
	}
	return s
}

func accountID(baseURL, orgID, userID string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	org := strings.TrimSpace(orgID)
	user := strings.TrimSpace(userID)
	if user != "" && org != "" {
		return org + ":" + user + "@" + base
	}
	if org != "" {
		return org + "@" + base
	}
	return base
}

func sanitizeOIDCLoginURL(baseURL, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.ContainsAny(trimmed, "\\\r\n\x00") {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return ""
	}
	resolved, err := base.Parse(trimmed)
	if err != nil || !strings.EqualFold(resolved.Scheme, base.Scheme) || !strings.EqualFold(resolved.Host, base.Host) {
		return ""
	}
	return trimmed
}

func (s *Service) AddOnAccountChanged(fn func(context.Context)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	s.onAccountChanged = append(s.onAccountChanged, fn)
	s.mu.Unlock()
}

func (s *Service) notifyAccountChanged(ctx context.Context) {
	s.mu.RLock()
	listeners := slices.Clone(s.onAccountChanged)
	s.mu.RUnlock()
	detached := context.WithoutCancel(ctx)
	for _, fn := range listeners {
		go fn(detached)
	}
}

func (s *Service) ActiveWorkspaceID() string {
	s.mu.RLock()
	id := s.activeWorkspaceID
	s.mu.RUnlock()
	if id == "" {
		return LocalWorkspaceID
	}
	return id
}

func (s *Service) CloudAuth(ctx context.Context) (string, string, bool) {
	acc, _, ok := s.activeAccountWithGen()
	if !ok || acc.BaseURL == "" || acc.Token == "" {
		return "", "", false
	}
	return acc.BaseURL, acc.Token, true
}

func (s *Service) CloudScope(ctx context.Context) *coordinator.CloudScope {
	acc, ok := s.accountForContext(ctx)
	if !ok || acc.OrgID == "" || acc.UserID == "" {
		return nil
	}
	return &coordinator.CloudScope{OrgID: acc.OrgID, UserID: acc.UserID}
}

func (s *Service) MCPServer(ctx context.Context) (mcpconfig.Server, bool) {
	acc, _, ok := s.activeAccountWithGen()
	if !ok || acc.BaseURL == "" || acc.Token == "" {
		return mcpconfig.Server{}, false
	}
	return mcpconfig.Server{
		ID:        CloudMCPServerID,
		Name:      CloudMCPServerID,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       acc.BaseURL + "/v1/mcp",
		Enabled:   true,
		Headers: []mcpconfig.Header{
			{Name: "Authorization", Value: "Bearer " + acc.Token},
		},
	}, true
}

func (s *Service) activeAccountWithGen() (storage.CloudAccount, uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	gen := s.accountGen
	if s.activeWorkspaceID == "" || s.activeWorkspaceID == LocalWorkspaceID {
		return storage.CloudAccount{}, gen, false
	}
	acc, ok := s.accounts[s.activeWorkspaceID]
	return acc, gen, ok
}

func (s *Service) accountForContext(ctx context.Context) (storage.CloudAccount, bool) {
	ws := sessioncontext.WorkspaceID(ctx)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ws != "" {
		if ws == LocalWorkspaceID {
			return storage.CloudAccount{}, false
		}
		acc, ok := s.accounts[ws]
		return acc, ok
	}
	if s.activeWorkspaceID == "" || s.activeWorkspaceID == LocalWorkspaceID {
		return storage.CloudAccount{}, false
	}
	acc, ok := s.accounts[s.activeWorkspaceID]
	return acc, ok
}

func (s *Service) currentGeneration() uint64 {
	s.mu.RLock()
	gen := s.accountGen
	s.mu.RUnlock()
	return gen
}

func (s *Service) isGeneration(gen uint64) bool {
	return s.currentGeneration() == gen
}

func (s *Service) cancelActiveSyncLocked() {
	s.accountGen++
	if s.activeSyncCancel != nil {
		s.activeSyncCancel()
	}
}

func (s *Service) clearEndpointCachesLocked() {
	clear(s.endpointCache)
}

func (s *Service) workspaceSummariesLocked() []WorkspaceSummary {
	out := make([]WorkspaceSummary, 0, len(s.accounts))
	for _, acc := range s.accounts {
		out = append(out, WorkspaceSummary{
			ID:           acc.ID,
			URL:          acc.BaseURL,
			OrgID:        acc.OrgID,
			OrgName:      acc.OrgName,
			Email:        acc.Email,
			DisplayName:  acc.DisplayName,
			Role:         acc.Role,
			OIDCLoginURL: acc.OIDCLoginURL,
			Capabilities: acc.CapabilitiesJSON,
			ConnectedAt:  acc.ConnectedAt,
			LastSyncAt:   acc.LastSyncAt,
			LastError:    acc.LastError,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ConnectedAt.Equal(out[j].ConnectedAt) {
			return out[i].ConnectedAt.Before(out[j].ConnectedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *Service) Status(_ context.Context) Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaces := s.workspaceSummariesLocked()
	activeID := s.activeWorkspaceID
	if activeID == "" {
		activeID = LocalWorkspaceID
	}
	if activeID == LocalWorkspaceID {
		return Status{
			ActiveWorkspaceID: LocalWorkspaceID,
			Connected:         false,
			Status:            "disconnected",
			Workspaces:        workspaces,
		}
	}
	acc, ok := s.accounts[activeID]
	if !ok {
		return Status{
			ActiveWorkspaceID: LocalWorkspaceID,
			Connected:         false,
			Status:            "disconnected",
			Workspaces:        workspaces,
		}
	}
	connected := acc.Token != ""
	st := "disconnected"
	if connected {
		st = "connected"
	}
	return Status{
		ActiveWorkspaceID: acc.ID,
		Connected:         connected,
		Status:            st,
		URL:               acc.BaseURL,
		User: &CloudUser{
			ID:          acc.UserID,
			UserID:      acc.UserID,
			OrgID:       acc.OrgID,
			OrgName:     acc.OrgName,
			Email:       acc.Email,
			DisplayName: acc.DisplayName,
			Role:        acc.Role,
		},
		OIDCLoginURL: acc.OIDCLoginURL,
		Capabilities: acc.CapabilitiesJSON,
		ConnectedAt:  acc.ConnectedAt,
		LastSyncAt:   acc.LastSyncAt,
		LastError:    acc.LastError,
		Workspaces:   workspaces,
	}
}

func (s *Service) Connect(ctx context.Context, req ConnectRequest) (Status, error) {
	baseURL, err := normalizeBaseURL(req.URL)
	if err != nil {
		return Status{}, err
	}
	code := strings.TrimSpace(req.Code)
	token := strings.TrimSpace(req.Token)
	if code == "" && token == "" {
		return Status{}, errors.New("code or token is required")
	}

	disc, discErr := s.client.discover(ctx, baseURL)
	oidcLoginURL := "/v1/auth/login"
	caps := json.RawMessage(`{}`)
	if discErr == nil {
		if safe := sanitizeOIDCLoginURL(baseURL, disc.OIDCLoginURL); safe != "" {
			oidcLoginURL = safe
		}
		if len(disc.Capabilities) > 0 {
			caps = disc.Capabilities
		}
	}

	var user CloudUser
	if code != "" {
		exch, err := s.client.exchangeOIDC(ctx, baseURL, code, strings.TrimSpace(req.CodeVerifier))
		if err != nil {
			return Status{}, err
		}
		token = exch.Token
		user = exch.User.toUser()
	}
	me, err := s.client.getMe(ctx, baseURL, token)
	if err != nil {
		return Status{}, err
	}
	if me.ID != "" {
		user = me
	}
	if user.ID == "" || user.OrgID == "" {
		return Status{}, errors.New("cloud server returned an incomplete user profile")
	}

	id := accountID(baseURL, user.OrgID, user.ID)

	// Cancel any in-flight sync and serialize account mutation under syncMu so
	// stale sync writes can never overwrite the new active workspace.
	s.mu.Lock()
	s.cancelActiveSyncLocked()
	s.mu.Unlock()

	s.syncMu.Lock()
	connectedAt := time.Now().UTC()
	s.mu.RLock()
	if existing, ok := s.accounts[id]; ok && !existing.ConnectedAt.IsZero() {
		connectedAt = existing.ConnectedAt
	}
	s.mu.RUnlock()

	acc := storage.CloudAccount{
		ID:               id,
		BaseURL:          baseURL,
		Token:            token,
		OIDCLoginURL:     oidcLoginURL,
		UserID:           user.ID,
		OrgID:            user.OrgID,
		OrgName:          user.OrgName,
		Email:            user.Email,
		DisplayName:      user.DisplayName,
		Role:             user.Role,
		CapabilitiesJSON: caps,
		ConnectedAt:      connectedAt,
	}
	if err := s.store.SaveCloudAccount(ctx, acc); err != nil {
		s.syncMu.Unlock()
		return Status{}, err
	}
	if err := s.store.SaveActiveWorkspaceID(ctx, id); err != nil {
		s.syncMu.Unlock()
		return Status{}, err
	}
	s.mu.Lock()
	s.accounts[id] = acc
	s.activeWorkspaceID = id
	s.clearEndpointCachesLocked()
	s.mu.Unlock()
	s.syncMu.Unlock()

	syncCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, _ = s.SyncNow(syncCtx)
	s.notifyAccountChanged(ctx)
	return s.Status(ctx), nil
}

func (s *Service) SwitchWorkspace(ctx context.Context, workspaceID string) (Status, error) {
	target := strings.TrimSpace(workspaceID)
	if target == "" {
		return Status{}, ErrNotFound
	}
	s.mu.Lock()
	if target != LocalWorkspaceID {
		if _, ok := s.accounts[target]; !ok {
			s.mu.Unlock()
			return Status{}, ErrNotFound
		}
	}
	if s.activeWorkspaceID == target {
		s.mu.Unlock()
		return s.Status(ctx), nil
	}
	s.cancelActiveSyncLocked()
	s.mu.Unlock()

	s.syncMu.Lock()
	s.mu.RLock()
	_, exists := s.accounts[target]
	s.mu.RUnlock()
	if target != LocalWorkspaceID && !exists {
		s.syncMu.Unlock()
		return Status{}, ErrNotFound
	}
	if err := s.store.SaveActiveWorkspaceID(ctx, target); err != nil {
		s.syncMu.Unlock()
		return Status{}, err
	}
	s.mu.Lock()
	s.activeWorkspaceID = target
	s.clearEndpointCachesLocked()
	s.mu.Unlock()
	s.syncMu.Unlock()

	if target != LocalWorkspaceID {
		s.TriggerSync()
	}
	s.notifyAccountChanged(ctx)
	return s.Status(ctx), nil
}

func (s *Service) Disconnect(ctx context.Context, workspaceID string) error {
	target := strings.TrimSpace(workspaceID)
	s.mu.Lock()
	if target == "" {
		target = s.activeWorkspaceID
	}
	if target == "" || target == LocalWorkspaceID {
		s.mu.Unlock()
		return nil
	}
	if _, existed := s.accounts[target]; !existed {
		s.mu.Unlock()
		return nil
	}
	if s.activeWorkspaceID == target {
		s.cancelActiveSyncLocked()
	}
	s.mu.Unlock()

	// Wait for any cancelled sync to unwind before deleting SQLite rows so it
	// can never resurrect rows after DeleteCloudAccount completes, and persist
	// before mutating in-memory state so DB errors never leave memory diverged.
	s.syncMu.Lock()
	s.mu.RLock()
	prev, existed := s.accounts[target]
	wasActive := s.activeWorkspaceID == target
	s.mu.RUnlock()
	if !existed {
		s.syncMu.Unlock()
		return nil
	}
	if err := s.store.DeleteCloudAccount(ctx, target); err != nil {
		s.syncMu.Unlock()
		return err
	}
	if wasActive {
		if err := s.store.SaveActiveWorkspaceID(ctx, LocalWorkspaceID); err != nil {
			s.syncMu.Unlock()
			return err
		}
	}
	s.mu.Lock()
	delete(s.accounts, target)
	if s.activeWorkspaceID == target {
		s.accountGen++
		s.activeWorkspaceID = LocalWorkspaceID
		s.clearEndpointCachesLocked()
	}
	s.mu.Unlock()
	s.syncMu.Unlock()

	if prev.BaseURL != "" && prev.Token != "" {
		go func(baseURL, token string) {
			logoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.client.logout(logoutCtx, baseURL, token)
		}(prev.BaseURL, prev.Token)
	}
	if wasActive {
		s.notifyAccountChanged(ctx)
	}
	return nil
}
