package cloudsync

import (
	"context"
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/storage"
)

type InstructionsStore struct {
	cloud    *Service
	fallback instructions.Store
}

func NewInstructionsStore(cloud *Service, fallback instructions.Store) instructions.Store {
	return &InstructionsStore{cloud: cloud, fallback: fallback}
}

func usesLocalFallback(key instructions.Key) bool {
	return key.Scope == instructions.ScopeBot || (key.Scope == instructions.ScopeUser && key.Kind == instructions.KindBots)
}

func resolveCloudInstructionKey(acc storage.CloudAccount, key instructions.Key) instructions.Key {
	orgID := key.OrgID
	if orgID == "" {
		orgID = acc.OrgID
	}
	scopeID := key.ScopeID
	if scopeID == "" {
		switch key.Scope {
		case instructions.ScopeOrg:
			scopeID = orgID
		case instructions.ScopeUser:
			scopeID = acc.UserID
		}
	}
	return instructions.Key{OrgID: orgID, Scope: key.Scope, ScopeID: scopeID, Kind: key.Kind}
}

func (s *InstructionsStore) Get(ctx context.Context, key instructions.Key) (instructions.Document, error) {
	if err := key.Validate(); err != nil {
		return instructions.Document{}, err
	}
	acc, ok := s.cloud.accountForContext(ctx)
	if !ok || usesLocalFallback(key) {
		return s.fallback.Get(ctx, key)
	}
	resolved := resolveCloudInstructionKey(acc, key)
	rec, found, err := s.cloud.store.GetCloudInstruction(ctx, acc.ID, resolved.OrgID, string(resolved.Scope), resolved.ScopeID, string(resolved.Kind))
	if err != nil {
		return instructions.Document{}, err
	}
	if !found {
		return instructions.Document{Key: resolved}, nil
	}
	return instructions.Document{
		Key:       instructions.Key{OrgID: rec.OrgID, Scope: resolved.Scope, ScopeID: rec.ScopeID, Kind: resolved.Kind},
		Content:   rec.BodyMD,
		Version:   rec.Version,
		UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (s *InstructionsStore) Save(ctx context.Context, key instructions.Key, content, expectedVersion string) (instructions.Document, error) {
	if err := key.Validate(); err != nil {
		return instructions.Document{}, err
	}
	acc, ok := s.cloud.accountForContext(ctx)
	if !ok || usesLocalFallback(key) {
		return s.fallback.Save(ctx, key, content, expectedVersion)
	}
	resolved := resolveCloudInstructionKey(acc, key)
	saved, err := s.cloud.saveInstructionForAccount(ctx, acc, 0, string(resolved.Scope), resolved.ScopeID, string(resolved.Kind), content, expectedVersion)
	if err != nil {
		var uerr *UpstreamError
		if errors.As(err, &uerr) && uerr.StatusCode == http.StatusConflict {
			cur, getErr := s.cloud.getInstructionForAccount(ctx, acc, 0, string(resolved.Scope), resolved.ScopeID, string(resolved.Kind))
			if getErr != nil {
				cur, _, _ = s.cloud.store.GetCloudInstruction(ctx, acc.ID, resolved.OrgID, string(resolved.Scope), resolved.ScopeID, string(resolved.Kind))
			}
			return instructions.Document{
				Key:       resolved,
				Content:   cur.BodyMD,
				Version:   cur.Version,
				UpdatedAt: cur.UpdatedAt,
			}, instructions.ErrConflict
		}
		return instructions.Document{}, err
	}
	return instructions.Document{
		Key:       instructions.Key{OrgID: saved.OrgID, Scope: resolved.Scope, ScopeID: saved.ScopeID, Kind: resolved.Kind},
		Content:   saved.BodyMD,
		Version:   saved.Version,
		UpdatedAt: saved.UpdatedAt,
	}, nil
}
