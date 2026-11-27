package cloudsync

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Service) Run(ctx context.Context) {
	if _, _, ok := s.activeAccountWithGen(); ok {
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
			if _, _, ok := s.activeAccountWithGen(); ok {
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

	client := s.clientFor(acc.ID)
	me, err := client.getMe(syncCtx, acc.BaseURL, s.latestToken(acc))
	if err != nil {
		if s.isGeneration(gen) {
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
		spaces, spErr = client.listSpaces(syncCtx, acc.BaseURL, s.latestToken(acc))
	}()
	go func() {
		defer wg.Done()
		threads, thErr = client.listThreads(syncCtx, acc.BaseURL, s.latestToken(acc))
	}()
	wg.Wait()
	if spErr != nil {
		if s.isGeneration(gen) {
			s.recordSyncError(syncCtx, gen, acc.ID, acc.LastSyncAt, spErr.Error())
		}
		return s.Status(ctx), spErr
	}
	if thErr != nil {
		if s.isGeneration(gen) {
			s.recordSyncError(syncCtx, gen, acc.ID, acc.LastSyncAt, thErr.Error())
		}
		return s.Status(ctx), thErr
	}
	if !s.isGeneration(gen) {
		return s.Status(ctx), context.Canceled
	}
	if err := s.store.ReplaceCloudSpaces(syncCtx, acc.ID, spaces); err != nil {
		return s.Status(ctx), err
	}
	threads = slices.DeleteFunc(threads, func(thread storage.CloudThread) bool {
		return thread.SourceType == nativeMCPSource
	})
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

	s.persistMu.Lock()
	if !s.isGeneration(gen) {
		s.persistMu.Unlock()
		return s.Status(ctx), context.Canceled
	}
	cur, ok := s.currentAccountToken(acc.ID)
	if !ok {
		s.persistMu.Unlock()
		return s.Status(ctx), context.Canceled
	}
	cur.OrgID = acc.OrgID
	cur.OrgName = acc.OrgName
	cur.UserID = acc.UserID
	cur.Email = acc.Email
	cur.DisplayName = acc.DisplayName
	cur.Role = acc.Role
	cur.LastSyncAt = time.Now().UTC()
	cur.LastError = ""
	if err := s.store.SaveCloudAccount(syncCtx, cur); err != nil {
		s.persistMu.Unlock()
		return s.Status(ctx), err
	}
	s.mu.Lock()
	if s.accountGen == gen {
		s.accounts[acc.ID] = cur
	}
	s.mu.Unlock()
	s.persistMu.Unlock()
	return s.Status(ctx), nil
}

func (s *Service) recordSyncError(ctx context.Context, gen uint64, accountID string, lastSync time.Time, message string) {
	if !s.isGeneration(gen) {
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
			doc, err := s.clientFor(acc.ID).getInstruction(ctx, acc.BaseURL, s.latestToken(acc), t.scope, t.scopeID, t.kind)
			if err != nil || !s.isGeneration(gen) {
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
		events, err := s.clientFor(acc.ID).listUsageEvents(ctx, acc.BaseURL, s.latestToken(acc), afterID, 500)
		if err != nil || len(events) == 0 || !s.isGeneration(gen) {
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
	paths := []string{"/v1/models", "/v1/machines", "/v1/usage/summary"}
	var wg sync.WaitGroup
	wg.Add(len(paths))
	for _, path := range paths {
		go func(p string) {
			defer wg.Done()
			if raw, _, err := s.clientFor(acc.ID).doJSON(ctx, http.MethodGet, acc.BaseURL, p, s.latestToken(acc), nil); err == nil {
				s.storeEndpointCache(gen, p, raw)
			}
		}(path)
	}
	wg.Wait()
}
