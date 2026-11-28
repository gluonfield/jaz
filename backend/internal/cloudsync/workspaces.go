package cloudsync

import (
	"context"
	"errors"

	"github.com/wins/jaz/backend/internal/storage"
)

var errCloudIdentityChanged = errors.New("Cloud credentials identify a different workspace or user")

func matchesAccount(acc storage.CloudAccount, user CloudUser) bool {
	return (acc.OrgID == "" || user.OrgID == "" || acc.OrgID == user.OrgID) &&
		(acc.UserID == "" || user.ID == "" || acc.UserID == user.ID)
}

type workspaceKey struct {
	server string
	orgID  string
}

func (s *Service) loadWorkspaces(ctx context.Context) {
	accounts, err := s.store.ListCloudAccounts(ctx)
	if err != nil {
		s.log.Warn("load cloud accounts failed", "error", err)
		return
	}
	activeID, err := s.store.LoadActiveWorkspaceID(ctx)
	if err != nil {
		s.log.Warn("load active workspace failed", "error", err)
	}

	winners := make(map[workspaceKey]storage.CloudAccount)
	for i, acc := range accounts {
		baseURL, err := normalizeBaseURL(acc.BaseURL)
		if err != nil {
			continue
		}
		acc.BaseURL = baseURL
		accounts[i] = acc
		if acc.OrgID == "" {
			continue
		}
		key := workspaceKey{server: acc.BaseURL, orgID: acc.OrgID}
		prev, found := winners[key]
		if !found || preferWorkspace(acc, prev, activeID) {
			winners[key] = acc
		}
	}
	var removed []string
	for _, acc := range accounts {
		winner := winners[workspaceKey{server: acc.BaseURL, orgID: acc.OrgID}]
		if winner.ID != "" && winner.ID != acc.ID {
			removed = append(removed, acc.ID)
			if acc.ID == activeID {
				s.activeWorkspaceID = winner.ID
			}
			continue
		}
		if acc.ID != "" {
			s.accounts[acc.ID] = acc
			if acc.ID == activeID {
				s.activeWorkspaceID = acc.ID
			}
		}
	}
	if len(removed) > 0 {
		if err := s.store.ApplyCloudWorkspaceChange(ctx, storage.CloudWorkspaceChange{ActiveID: s.activeWorkspaceID, RemoveIDs: removed}); err != nil {
			s.log.Warn("persist cloud workspace cleanup failed", "error", err)
		}
	}
}

func preferWorkspace(candidate, current storage.CloudAccount, activeID string) bool {
	if (candidate.Token == "") != (current.Token == "") {
		return candidate.Token != ""
	}
	if candidate.ID == activeID || current.ID == activeID {
		return candidate.ID == activeID
	}
	return candidate.ConnectedAt.After(current.ConnectedAt)
}

func (s *Service) activateAccount(ctx context.Context, acc storage.CloudAccount) error {
	s.mu.Lock()
	s.cancelActiveSyncLocked()
	s.mu.Unlock()

	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	// Keep local caches scoped to the signed-in user when a workspace changes hands.
	acc.ID = acc.OrgID + ":" + acc.UserID + "@" + acc.BaseURL
	var replaced []string
	s.mu.RLock()
	for _, existing := range s.accounts {
		if existing.BaseURL != acc.BaseURL || existing.OrgID != acc.OrgID {
			continue
		}
		if existing.UserID == acc.UserID {
			acc.ID = existing.ID
		} else {
			replaced = append(replaced, existing.ID)
		}
	}
	s.mu.RUnlock()
	if err := s.store.ApplyCloudWorkspaceChange(ctx, storage.CloudWorkspaceChange{ActiveID: acc.ID, SignIn: &acc, RemoveIDs: replaced}); err != nil {
		return err
	}
	s.mu.Lock()
	for _, id := range replaced {
		delete(s.accounts, id)
	}
	s.accounts[acc.ID] = acc
	s.activeWorkspaceID = acc.ID
	s.clearEndpointCachesLocked()
	s.mu.Unlock()
	return nil
}
