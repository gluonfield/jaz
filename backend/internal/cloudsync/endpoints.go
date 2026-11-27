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
	raw, _, err := s.clientFor(acc.ID).doJSON(ctx, http.MethodGet, acc.BaseURL, path, acc.Token, nil)
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
	if acc.Token != "" {
		if remote, err := s.clientFor(acc.ID).listSpaces(ctx, acc.BaseURL, acc.Token); err == nil {
			if s.isGeneration(gen) {
				_ = s.store.ReplaceCloudSpaces(ctx, acc.ID, remote)
			}
			return remote, nil
		}
	}
	spaces, err := s.store.ListCloudSpaces(ctx, acc.ID)
	if err != nil {
		return nil, err
	}
	return spaces, nil
}

func (s *Service) CreateSpace(ctx context.Context, req CreateSpaceRequest) (storage.CloudSpace, error) {
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return storage.CloudSpace{}, ErrNotConnected
	}
	created, err := s.clientFor(acc.ID).createSpace(ctx, acc.BaseURL, acc.Token, req)
	if err != nil {
		return storage.CloudSpace{}, err
	}
	created.AccountID = acc.ID
	if s.isGeneration(gen) {
		_ = s.store.UpsertCloudSpace(ctx, acc.ID, created)
	}
	return created, nil
}

func (s *Service) PatchSpace(ctx context.Context, spaceID string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(spaceID); err != nil {
		return nil, 0, err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	raw, status, err := s.clientFor(acc.ID).doJSON(ctx, http.MethodPatch, acc.BaseURL, "/v1/spaces/"+url.PathEscape(spaceID), acc.Token, body)
	if err != nil {
		return nil, status, err
	}
	var updated storage.CloudSpace
	if json.Unmarshal(raw, &updated) == nil && updated.ID != "" && s.isGeneration(gen) {
		updated.AccountID = acc.ID
		_ = s.store.UpsertCloudSpace(ctx, acc.ID, updated)
	}
	return raw, status, nil
}

func (s *Service) AddSpaceMember(ctx context.Context, spaceID string, body json.RawMessage) (json.RawMessage, int, error) {
	return s.authedSegmentJSON(ctx, http.MethodPost, "/v1/spaces/", spaceID, "/members", body)
}

func (s *Service) GetSpace(ctx context.Context, spaceID string) (json.RawMessage, int, error) {
	return s.authedSegmentJSON(ctx, http.MethodGet, "/v1/spaces/", spaceID, "", nil)
}

func (s *Service) DeleteSpace(ctx context.Context, spaceID string) (json.RawMessage, int, error) {
	if err := validatePathSegment(spaceID); err != nil {
		return nil, 0, err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	raw, status, err := s.clientFor(acc.ID).doJSON(ctx, http.MethodDelete, acc.BaseURL, "/v1/spaces/"+url.PathEscape(spaceID), acc.Token, nil)
	if err != nil {
		return nil, status, err
	}
	if s.isGeneration(gen) {
		if spaces, listErr := s.store.ListCloudSpaces(ctx, acc.ID); listErr == nil {
			filtered := make([]storage.CloudSpace, 0, len(spaces))
			for _, sp := range spaces {
				if sp.ID != spaceID {
					filtered = append(filtered, sp)
				}
			}
			_ = s.store.ReplaceCloudSpaces(ctx, acc.ID, filtered)
		}
		s.TriggerSync()
	}
	return raw, status, nil
}

func (s *Service) OpenSpaceSourceContent(ctx context.Context, spaceID, sourceID string) (*http.Response, error) {
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, err
	}
	return s.clientFor(acc.ID).openSpaceSourceContent(ctx, acc.BaseURL, acc.Token, spaceID, sourceID)
}

func (s *Service) GetInstruction(ctx context.Context, scope, scopeID, kind string) (storage.CloudInstruction, error) {
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return storage.CloudInstruction{}, ErrNotConnected
	}
	return s.getInstructionForAccount(ctx, acc, gen, scope, scopeID, kind)
}

func (s *Service) getInstructionForAccount(ctx context.Context, acc storage.CloudAccount, gen uint64, scope, scopeID, kind string) (storage.CloudInstruction, error) {
	doc, err := s.clientFor(acc.ID).getInstruction(ctx, acc.BaseURL, s.latestToken(acc), scope, scopeID, kind)
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
	saved, err := s.clientFor(acc.ID).saveInstruction(ctx, acc.BaseURL, s.latestToken(acc), scope, scopeID, kind, bodyMD, expectedVersion)
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
	return s.authedSegmentJSON(ctx, http.MethodPost, "/v1/runs/", runID, "/uploads/presign", body)
}

func (s *Service) CommitUpload(ctx context.Context, runID string, body json.RawMessage) (json.RawMessage, int, error) {
	return s.authedSegmentJSON(ctx, http.MethodPost, "/v1/runs/", runID, "/uploads/commit", body)
}

func (s *Service) OpenRunFile(ctx context.Context, runID, relpath string) (*http.Response, error) {
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, err
	}
	return s.clientFor(acc.ID).openRunFile(ctx, acc.BaseURL, acc.Token, runID, relpath)
}
