package cloudsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Service) storeEndpointCache(gen uint64, path string, raw json.RawMessage) {
	s.mu.Lock()
	if s.accountGen == gen {
		s.endpointCache[path] = cachedEndpoint{raw: raw, at: time.Now()}
	}
	s.mu.Unlock()
}

func (s *Service) fetchCachedEndpoint(ctx context.Context, path string, ttl time.Duration, disconnectedFallback json.RawMessage) (json.RawMessage, error) {
	s.mu.RLock()
	gen := s.accountGen
	var acc storage.CloudAccount
	var ok bool
	if s.activeWorkspaceID != "" && s.activeWorkspaceID != LocalWorkspaceID {
		acc, ok = s.accounts[s.activeWorkspaceID]
	}
	cached := s.endpointCache[path]
	s.mu.RUnlock()

	if !ok || acc.Token == "" {
		if disconnectedFallback != nil {
			return disconnectedFallback, nil
		}
		return nil, ErrNotConnected
	}
	if ttl > 0 && len(cached.raw) > 0 && time.Since(cached.at) < ttl {
		return cached.raw, nil
	}
	raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, path, acc.Token, nil)
	if err != nil {
		if len(cached.raw) > 0 && s.isGeneration(gen) {
			return cached.raw, nil
		}
		return nil, err
	}
	s.storeEndpointCache(gen, path, raw)
	return raw, nil
}

func (s *Service) ListModels(ctx context.Context) (json.RawMessage, error) {
	return s.fetchCachedEndpoint(ctx, "/v1/models", cacheTTL, json.RawMessage(`{"models":[]}`))
}

func (s *Service) ListMachines(ctx context.Context) (json.RawMessage, error) {
	return s.fetchCachedEndpoint(ctx, "/v1/machines", 0, json.RawMessage(`{"machines":[]}`))
}

func (s *Service) GetUsageSummary(ctx context.Context) (json.RawMessage, error) {
	return s.fetchCachedEndpoint(ctx, "/v1/usage/summary", 0, nil)
}

func (s *Service) ListSpaces(ctx context.Context) ([]storage.CloudSpace, error) {
	acc, gen, ok := s.activeAccountWithGen()
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
	if s.isGeneration(gen) {
		_ = s.store.ReplaceCloudSpaces(ctx, acc.ID, remote)
	}
	return remote, nil
}

func (s *Service) CreateSpace(ctx context.Context, name string) (storage.CloudSpace, error) {
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return storage.CloudSpace{}, ErrNotConnected
	}
	created, err := s.client.createSpace(ctx, acc.BaseURL, acc.Token, name)
	if err != nil {
		return storage.CloudSpace{}, err
	}
	created.AccountID = acc.ID
	if s.isGeneration(gen) {
		_ = s.store.UpsertCloudSpace(ctx, acc.ID, created)
	}
	return created, nil
}

func (s *Service) AddSpaceMember(ctx context.Context, spaceID string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(spaceID); err != nil {
		return nil, 0, err
	}
	acc, _, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	return s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/spaces/"+url.PathEscape(spaceID)+"/members", acc.Token, body)
}

func (s *Service) GetInstruction(ctx context.Context, scope, scopeID, kind string) (storage.CloudInstruction, error) {
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return storage.CloudInstruction{}, ErrNotConnected
	}
	return s.getInstructionForAccount(ctx, acc, gen, scope, scopeID, kind)
}

func (s *Service) getInstructionForAccount(ctx context.Context, acc storage.CloudAccount, gen uint64, scope, scopeID, kind string) (storage.CloudInstruction, error) {
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
	if gen == 0 || s.isGeneration(gen) {
		_ = s.store.UpsertCloudInstruction(ctx, acc.ID, doc)
	}
	return doc, nil
}

func (s *Service) SaveInstruction(ctx context.Context, scope, scopeID, kind, bodyMD, expectedVersion string) (storage.CloudInstruction, error) {
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return storage.CloudInstruction{}, ErrNotConnected
	}
	return s.saveInstructionForAccount(ctx, acc, gen, scope, scopeID, kind, bodyMD, expectedVersion)
}

func (s *Service) saveInstructionForAccount(ctx context.Context, acc storage.CloudAccount, gen uint64, scope, scopeID, kind, bodyMD, expectedVersion string) (storage.CloudInstruction, error) {
	saved, err := s.client.saveInstruction(ctx, acc.BaseURL, acc.Token, scope, scopeID, kind, bodyMD, expectedVersion)
	if err != nil {
		return storage.CloudInstruction{}, err
	}
	if saved.OrgID == "" {
		saved.OrgID = acc.OrgID
	}
	if gen == 0 || s.isGeneration(gen) {
		_ = s.store.UpsertCloudInstruction(ctx, acc.ID, saved)
	}
	return saved, nil
}

func (s *Service) PresignUpload(ctx context.Context, runID string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(runID); err != nil {
		return nil, 0, err
	}
	acc, _, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	return s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/runs/"+url.PathEscape(runID)+"/uploads/presign", acc.Token, body)
}

func (s *Service) CommitUpload(ctx context.Context, runID string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(runID); err != nil {
		return nil, 0, err
	}
	acc, _, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	return s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/runs/"+url.PathEscape(runID)+"/uploads/commit", acc.Token, body)
}

func (s *Service) OpenRunFile(ctx context.Context, runID, relpath string) (*http.Response, error) {
	acc, _, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, ErrNotConnected
	}
	return s.client.openRunFile(ctx, acc.BaseURL, acc.Token, runID, relpath)
}
