package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	defaultSyncInterval = 30 * time.Second
	cacheTTL            = 30 * time.Second
)

type Service struct {
	store  storage.CloudMirrorStore
	client *cloudClient
	log    *log.Logger

	mu                sync.RWMutex
	accounts          map[string]storage.CloudAccount
	activeWorkspaceID string
	accountGen        uint64
	activeSyncCancel  context.CancelFunc
	modelsRaw         json.RawMessage
	modelsAt          time.Time
	machinesRaw       json.RawMessage
	machinesAt        time.Time
	usageSummaryRaw   json.RawMessage
	usageSummaryAt    time.Time
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
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.BaseURL == "" || acc.Token == "" {
		return "", "", false
	}
	return acc.BaseURL, acc.Token, true
}

func (s *Service) activeAccount(_ context.Context) (storage.CloudAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
		if disc.OIDCLoginURL != "" {
			oidcLoginURL = disc.OIDCLoginURL
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
	s.accountGen++
	if s.activeSyncCancel != nil {
		s.activeSyncCancel()
	}
	s.mu.Unlock()

	s.syncMu.Lock()
	now := time.Now().UTC()
	connectedAt := now
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
	s.accountGen++
	if s.activeSyncCancel != nil {
		s.activeSyncCancel()
	}
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
	prev, existed := s.accounts[target]
	if !existed {
		s.mu.Unlock()
		return nil
	}
	wasActive := s.activeWorkspaceID == target
	delete(s.accounts, target)
	if wasActive {
		s.accountGen++
		s.activeWorkspaceID = LocalWorkspaceID
		s.clearEndpointCachesLocked()
		if s.activeSyncCancel != nil {
			s.activeSyncCancel()
		}
	}
	s.mu.Unlock()

	// Wait for any cancelled sync to unwind before deleting SQLite rows so it
	// can never resurrect rows after DeleteCloudAccount completes.
	s.syncMu.Lock()
	err := s.store.DeleteCloudAccount(ctx, target)
	if err == nil && wasActive {
		err = s.store.SaveActiveWorkspaceID(ctx, LocalWorkspaceID)
	}
	s.syncMu.Unlock()
	if err != nil {
		return err
	}

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

func (s *Service) clearEndpointCachesLocked() {
	s.modelsRaw, s.machinesRaw, s.usageSummaryRaw = nil, nil, nil
	s.modelsAt, s.machinesAt, s.usageSummaryAt = time.Time{}, time.Time{}, time.Time{}
}

func (s *Service) Run(ctx context.Context) {
	if _, ok := s.activeAccount(ctx); ok {
		_, _ = s.SyncNow(ctx)
	}
	ticker := time.NewTicker(defaultSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
			_, _ = s.SyncNow(ctx)
		case <-ticker.C:
			if _, ok := s.activeAccount(ctx); ok {
				_, _ = s.SyncNow(ctx)
			}
		}
	}
}

func (s *Service) TriggerSync() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) SyncNow(ctx context.Context) (Status, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	s.mu.Lock()
	if s.activeWorkspaceID == "" || s.activeWorkspaceID == LocalWorkspaceID {
		s.mu.Unlock()
		return s.Status(ctx), ErrNotConnected
	}
	acc, ok := s.accounts[s.activeWorkspaceID]
	if !ok || acc.BaseURL == "" || acc.Token == "" {
		s.mu.Unlock()
		return s.Status(ctx), ErrNotConnected
	}
	gen := s.accountGen
	syncCtx, cancel := context.WithCancel(ctx)
	s.activeSyncCancel = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.activeSyncCancel = nil
		s.mu.Unlock()
	}()

	me, err := s.client.getMe(syncCtx, acc.BaseURL, acc.Token)
	if err != nil {
		if s.currentGeneration() == gen {
			s.recordSyncError(syncCtx, gen, acc.ID, acc.LastSyncAt, err.Error())
		}
		return s.Status(ctx), err
	}
	acc.OrgID = me.OrgID
	acc.OrgName = me.OrgName
	acc.UserID = me.ID
	acc.Email = me.Email
	acc.DisplayName = me.DisplayName
	acc.Role = me.Role

	var (
		spaces  []storage.CloudSpace
		threads []storage.CloudThread
		spErr   error
		thErr   error
		wg      sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		spaces, spErr = s.client.listSpaces(syncCtx, acc.BaseURL, acc.Token)
	}()
	go func() {
		defer wg.Done()
		threads, thErr = s.client.listThreads(syncCtx, acc.BaseURL, acc.Token)
	}()
	wg.Wait()
	if spErr != nil {
		if s.currentGeneration() == gen {
			s.recordSyncError(syncCtx, gen, acc.ID, acc.LastSyncAt, spErr.Error())
		}
		return s.Status(ctx), spErr
	}
	if thErr != nil {
		if s.currentGeneration() == gen {
			s.recordSyncError(syncCtx, gen, acc.ID, acc.LastSyncAt, thErr.Error())
		}
		return s.Status(ctx), thErr
	}
	if s.currentGeneration() != gen {
		return s.Status(ctx), context.Canceled
	}
	if err := s.store.ReplaceCloudSpaces(syncCtx, acc.ID, spaces); err != nil {
		return s.Status(ctx), err
	}
	if err := s.store.SyncCloudThreads(syncCtx, acc.ID, threads); err != nil {
		return s.Status(ctx), err
	}

	var auxWG sync.WaitGroup
	auxWG.Add(3)
	go func() {
		defer auxWG.Done()
		s.syncInstructions(syncCtx, gen, acc, spaces)
	}()
	go func() {
		defer auxWG.Done()
		s.syncUsageEvents(syncCtx, gen, acc)
	}()
	go func() {
		defer auxWG.Done()
		s.refreshCachedEndpoints(syncCtx, gen, acc)
	}()
	auxWG.Wait()

	if s.currentGeneration() != gen {
		return s.Status(ctx), context.Canceled
	}
	now := time.Now().UTC()
	acc.LastSyncAt = now
	acc.LastError = ""
	if err := s.store.SaveCloudAccount(syncCtx, acc); err != nil {
		return s.Status(ctx), err
	}
	s.mu.Lock()
	if s.accountGen == gen {
		s.accounts[acc.ID] = acc
	}
	s.mu.Unlock()
	return s.Status(ctx), nil
}

func (s *Service) recordSyncError(ctx context.Context, gen uint64, accountID string, lastSync time.Time, message string) {
	if s.currentGeneration() != gen {
		return
	}
	_ = s.store.UpdateCloudSyncState(ctx, accountID, lastSync, message)
	s.mu.Lock()
	if s.accountGen == gen {
		if cur, ok := s.accounts[accountID]; ok {
			cur.LastError = message
			s.accounts[accountID] = cur
		}
	}
	s.mu.Unlock()
}

func (s *Service) syncInstructions(ctx context.Context, gen uint64, acc storage.CloudAccount, spaces []storage.CloudSpace) {
	type target struct{ scope, scopeID, kind string }
	targets := []target{
		{"org", acc.OrgID, "agents"},
		{"user", acc.UserID, "agents"},
		{"user", acc.UserID, "long_term"},
		{"user", acc.UserID, "short_term"},
	}
	for _, sp := range spaces {
		targets = append(targets, target{"space", sp.ID, "agents"})
	}
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, t := range targets {
		if t.scopeID == "" {
			continue
		}
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			doc, err := s.client.getInstruction(ctx, acc.BaseURL, acc.Token, t.scope, t.scopeID, t.kind)
			if err != nil || s.currentGeneration() != gen {
				return
			}
			if doc.OrgID == "" {
				doc.OrgID = acc.OrgID
			}
			_ = s.store.UpsertCloudInstruction(ctx, acc.ID, doc)
		}(t)
	}
	wg.Wait()
}

func (s *Service) syncUsageEvents(ctx context.Context, gen uint64, acc storage.CloudAccount) {
	afterID, err := s.store.MaxCloudUsageEventID(ctx, acc.ID)
	if err != nil {
		return
	}
	for range 10 {
		events, err := s.client.listUsageEvents(ctx, acc.BaseURL, acc.Token, afterID, 500)
		if err != nil || len(events) == 0 || s.currentGeneration() != gen {
			return
		}
		if err := s.store.UpsertCloudUsageEvents(ctx, acc.ID, events); err != nil {
			return
		}
		for _, ev := range events {
			if ev.ID > afterID {
				afterID = ev.ID
			}
		}
		if len(events) < 500 {
			return
		}
	}
}

func (s *Service) refreshCachedEndpoints(ctx context.Context, gen uint64, acc storage.CloudAccount) {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, "/v1/models", acc.Token, nil); err == nil {
			s.mu.Lock()
			if s.accountGen == gen {
				s.modelsRaw, s.modelsAt = raw, time.Now()
			}
			s.mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		if raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, "/v1/machines", acc.Token, nil); err == nil {
			s.mu.Lock()
			if s.accountGen == gen {
				s.machinesRaw, s.machinesAt = raw, time.Now()
			}
			s.mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		if raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, "/v1/usage/summary", acc.Token, nil); err == nil {
			s.mu.Lock()
			if s.accountGen == gen {
				s.usageSummaryRaw, s.usageSummaryAt = raw, time.Now()
			}
			s.mu.Unlock()
		}
	}()
	wg.Wait()
}

func (s *Service) ListSessions(ctx context.Context, archived bool) ([]storage.Session, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok {
		return nil, nil
	}
	threads, err := s.store.ListCloudThreads(ctx, acc.ID, archived)
	if err != nil {
		return nil, err
	}
	out := make([]storage.Session, 0, len(threads))
	for _, th := range threads {
		out = append(out, th.ToSession())
	}
	return out, nil
}

func (s *Service) GetSession(ctx context.Context, id string) (storage.Session, bool, error) {
	acc, connected := s.activeAccount(ctx)
	if !connected {
		return storage.Session{}, false, nil
	}
	th, ok, err := s.store.GetCloudThread(ctx, acc.ID, id)
	if err != nil {
		return storage.Session{}, false, err
	}
	if ok {
		return th.ToSession(), true, nil
	}
	if acc.Token == "" {
		return storage.Session{}, false, nil
	}
	remote, err := s.client.getThread(ctx, acc.BaseURL, acc.Token, id)
	if err != nil {
		return storage.Session{}, false, nil
	}
	remote.AccountID = acc.ID
	_ = s.store.UpsertCloudThread(ctx, acc.ID, remote)
	return remote.ToSession(), true, nil
}

func (s *Service) CreateThread(ctx context.Context, req CreateThreadRequest) (storage.Session, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return storage.Session{}, ErrNotConnected
	}
	created, err := s.client.createThread(ctx, acc.BaseURL, acc.Token, req)
	if err != nil {
		return storage.Session{}, err
	}
	created.AccountID = acc.ID
	if err := s.store.UpsertCloudThread(ctx, acc.ID, created); err != nil {
		return storage.Session{}, err
	}
	return created.ToSession(), nil
}

func (s *Service) LoadTranscript(ctx context.Context, id string) (ProjectedTranscript, bool, error) {
	acc, connected := s.activeAccount(ctx)
	if !connected {
		return ProjectedTranscript{}, false, nil
	}
	th, ok, err := s.store.GetCloudThread(ctx, acc.ID, id)
	if err != nil {
		return ProjectedTranscript{}, false, err
	}
	if !ok {
		if acc.Token == "" {
			return ProjectedTranscript{}, false, nil
		}
		remote, fetchErr := s.client.getThread(ctx, acc.BaseURL, acc.Token, id)
		if fetchErr != nil {
			return ProjectedTranscript{}, false, nil
		}
		remote.AccountID = acc.ID
		th = remote
		_ = s.store.UpsertCloudThread(ctx, acc.ID, th)
	}
	if acc.Token != "" {
		var (
			remote    storage.CloudThread
			remoteErr error
			fresh     []storage.CloudThreadEvent
			freshErr  error
			wg        sync.WaitGroup
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			remote, remoteErr = s.client.getThread(ctx, acc.BaseURL, acc.Token, th.ID)
		}()
		go func() {
			defer wg.Done()
			fresh, freshErr = s.client.fetchEventsOnce(ctx, acc.BaseURL, acc.Token, th.ID, th.LastSeq)
		}()
		wg.Wait()
		if remoteErr == nil {
			remote.AccountID = acc.ID
			remote.LastSeq = max(remote.LastSeq, th.LastSeq)
			th = remote
			_ = s.store.UpsertCloudThread(ctx, acc.ID, th)
		}
		if freshErr == nil && len(fresh) > 0 {
			_ = s.store.AppendCloudThreadEvents(ctx, acc.ID, th.ID, fresh)
		}
	}
	events, err := s.store.ListCloudThreadEvents(ctx, acc.ID, th.ID, 0)
	if err != nil {
		return ProjectedTranscript{}, true, err
	}
	return ProjectTranscript(th, events), true, nil
}

func (s *Service) StreamEvents(ctx context.Context, threadID string, afterSeq int64, emit func(sessionevents.Event)) error {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return ErrNotConnected
	}
	projector := NewStreamProjector(threadID)
	allCached, err := s.store.ListCloudThreadEvents(ctx, acc.ID, threadID, 0)
	if err != nil {
		return err
	}
	lastSeq := afterSeq
	for _, ev := range allCached {
		if ev.Seq <= afterSeq {
			// Seed streaming assistant buffer for mid-turn reconnects without re-emitting.
			_ = projector.Convert(ev)
			continue
		}
		if ev.Seq > lastSeq {
			lastSeq = ev.Seq
		}
		emit(projector.Convert(ev))
	}
	return s.client.streamEvents(ctx, acc.BaseURL, acc.Token, threadID, lastSeq, func(ev storage.CloudThreadEvent) error {
		if ev.Seq > 0 && ev.Seq <= lastSeq {
			return nil
		}
		if ev.Seq > lastSeq {
			lastSeq = ev.Seq
		}
		_ = s.store.AppendCloudThreadEvents(ctx, acc.ID, threadID, []storage.CloudThreadEvent{ev})
		if term := cloudTerminalState(ev.Type); term != "" {
			status := storage.StatusIdle
			if term == "failed" {
				status = storage.StatusError
			} else if term == "cancelled" {
				status = storage.StatusInterrupted
			}
			_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, threadID, status, ev.CreatedAt)
			s.TriggerSync()
		} else if isCloudUserEvent(ev.Type) || isCloudAssistantDelta(ev.Type) {
			_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, threadID, storage.StatusRunning, ev.CreatedAt)
		}
		emit(projector.Convert(ev))
		return nil
	})
}

func (s *Service) CreateTurn(ctx context.Context, threadID string, body json.RawMessage) (json.RawMessage, int, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	raw, code, err := s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/threads/"+url.PathEscape(threadID)+"/turns", acc.Token, body)
	if err != nil {
		return nil, code, err
	}
	_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, threadID, storage.StatusRunning, time.Now().UTC())
	return raw, code, nil
}

func (s *Service) UpdateUserState(ctx context.Context, threadID string, patch storage.CloudThreadUserStatePatch) (storage.Session, json.RawMessage, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return storage.Session{}, nil, ErrNotConnected
	}
	payload := map[string]any{}
	if patch.Pinned != nil {
		payload["pinned"] = *patch.Pinned
	}
	if patch.Archived != nil {
		payload["archived"] = *patch.Archived
	}
	if patch.PersonalSourcesEnabled != nil {
		payload["personal_sources_enabled"] = *patch.PersonalSourcesEnabled
	}
	if patch.Unread != nil && !*patch.Unread {
		if cur, found, err := s.store.GetCloudThread(ctx, acc.ID, threadID); err == nil && found && cur.LastSeq > 0 {
			payload["last_read_seq"] = cur.LastSeq
		}
	}
	raw, _, err := s.client.doJSON(ctx, http.MethodPut, acc.BaseURL, "/v1/threads/"+url.PathEscape(threadID)+"/user-state", acc.Token, payload)
	if err != nil {
		return storage.Session{}, nil, err
	}
	updated, err := s.store.UpdateCloudThreadUserState(ctx, acc.ID, threadID, patch)
	if err != nil {
		return storage.Session{}, raw, nil
	}
	return updated.ToSession(), raw, nil
}

func (s *Service) ListSpaces(ctx context.Context) ([]storage.CloudSpace, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok {
		return []storage.CloudSpace{}, nil
	}
	spaces, err := s.store.ListCloudSpaces(ctx, acc.ID)
	if err != nil {
		return nil, err
	}
	if len(spaces) > 0 || acc.Token == "" {
		return spaces, nil
	}
	remote, err := s.client.listSpaces(ctx, acc.BaseURL, acc.Token)
	if err != nil {
		return spaces, nil
	}
	_ = s.store.ReplaceCloudSpaces(ctx, acc.ID, remote)
	return remote, nil
}

func (s *Service) CreateSpace(ctx context.Context, name string) (storage.CloudSpace, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return storage.CloudSpace{}, ErrNotConnected
	}
	created, err := s.client.createSpace(ctx, acc.BaseURL, acc.Token, name)
	if err != nil {
		return storage.CloudSpace{}, err
	}
	created.AccountID = acc.ID
	_ = s.store.UpsertCloudSpace(ctx, acc.ID, created)
	return created, nil
}

func (s *Service) AddSpaceMember(ctx context.Context, spaceID string, body json.RawMessage) (json.RawMessage, int, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	return s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/spaces/"+url.PathEscape(spaceID)+"/members", acc.Token, body)
}

func (s *Service) ListModels(ctx context.Context) (json.RawMessage, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return json.RawMessage(`{"models":[]}`), nil
	}
	s.mu.RLock()
	cached, at, gen := s.modelsRaw, s.modelsAt, s.accountGen
	s.mu.RUnlock()
	if len(cached) > 0 && time.Since(at) < cacheTTL {
		return cached, nil
	}
	raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, "/v1/models", acc.Token, nil)
	if err != nil {
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}
	s.mu.Lock()
	if s.accountGen == gen {
		s.modelsRaw, s.modelsAt = raw, time.Now()
	}
	s.mu.Unlock()
	return raw, nil
}

func (s *Service) ListMachines(ctx context.Context) (json.RawMessage, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return json.RawMessage(`{"machines":[]}`), nil
	}
	gen := s.currentGeneration()
	raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, "/v1/machines", acc.Token, nil)
	if err != nil {
		s.mu.RLock()
		cached := s.machinesRaw
		s.mu.RUnlock()
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}
	s.mu.Lock()
	if s.accountGen == gen {
		s.machinesRaw, s.machinesAt = raw, time.Now()
	}
	s.mu.Unlock()
	return raw, nil
}

func (s *Service) GetUsageSummary(ctx context.Context) (json.RawMessage, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return nil, ErrNotConnected
	}
	gen := s.currentGeneration()
	raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, "/v1/usage/summary", acc.Token, nil)
	if err != nil {
		s.mu.RLock()
		cached := s.usageSummaryRaw
		s.mu.RUnlock()
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}
	s.mu.Lock()
	if s.accountGen == gen {
		s.usageSummaryRaw, s.usageSummaryAt = raw, time.Now()
	}
	s.mu.Unlock()
	return raw, nil
}

func (s *Service) GetInstruction(ctx context.Context, scope, scopeID, kind string) (storage.CloudInstruction, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return storage.CloudInstruction{}, ErrNotConnected
	}
	doc, err := s.client.getInstruction(ctx, acc.BaseURL, acc.Token, scope, scopeID, kind)
	if err != nil {
		if cached, found, cacheErr := s.store.GetCloudInstruction(ctx, acc.ID, acc.OrgID, scope, scopeID, kind); cacheErr == nil && found {
			return cached, nil
		}
		return storage.CloudInstruction{}, err
	}
	if doc.OrgID == "" {
		doc.OrgID = acc.OrgID
	}
	_ = s.store.UpsertCloudInstruction(ctx, acc.ID, doc)
	return doc, nil
}

func (s *Service) SaveInstruction(ctx context.Context, scope, scopeID, kind, bodyMD, expectedVersion string) (storage.CloudInstruction, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return storage.CloudInstruction{}, ErrNotConnected
	}
	saved, err := s.client.saveInstruction(ctx, acc.BaseURL, acc.Token, scope, scopeID, kind, bodyMD, expectedVersion)
	if err != nil {
		return storage.CloudInstruction{}, err
	}
	if saved.OrgID == "" {
		saved.OrgID = acc.OrgID
	}
	_ = s.store.UpsertCloudInstruction(ctx, acc.ID, saved)
	return saved, nil
}

func (s *Service) PresignUpload(ctx context.Context, runID string, body json.RawMessage) (json.RawMessage, int, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	return s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/runs/"+url.PathEscape(runID)+"/uploads/presign", acc.Token, body)
}

func (s *Service) CommitUpload(ctx context.Context, runID string, body json.RawMessage) (json.RawMessage, int, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	return s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/runs/"+url.PathEscape(runID)+"/uploads/commit", acc.Token, body)
}

func (s *Service) OpenRunFile(ctx context.Context, runID, relpath string) (*http.Response, error) {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.Token == "" {
		return nil, ErrNotConnected
	}
	return s.client.openRunFile(ctx, acc.BaseURL, acc.Token, runID, relpath)
}
