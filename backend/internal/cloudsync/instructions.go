package cloudsync

import (
	"context"
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/instructions"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

const CloudMCPServerID = "jaz_cloud"

type InstructionsStore struct {
	cloud    *Service
	fallback instructions.Store
}

func NewInstructionsStore(cloud *Service, fallback instructions.Store) instructions.Store {
	return &InstructionsStore{cloud: cloud, fallback: fallback}
}

func (s *InstructionsStore) Get(ctx context.Context, key instructions.Key) (instructions.Document, error) {
	if err := key.Validate(); err != nil {
		return instructions.Document{}, err
	}
	acc, ok := s.cloud.activeAccount(ctx)
	if !ok || key.Scope == instructions.ScopeBot {
		return s.fallback.Get(ctx, key)
	}
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
	if key.Scope == instructions.ScopeUser && key.Kind == instructions.KindBots {
		return s.fallback.Get(ctx, key)
	}
	rec, found, err := s.cloud.store.GetCloudInstruction(ctx, orgID, string(key.Scope), scopeID, string(key.Kind))
	if err != nil {
		return instructions.Document{}, err
	}
	if !found {
		return instructions.Document{Key: instructions.Key{OrgID: orgID, Scope: key.Scope, ScopeID: scopeID, Kind: key.Kind}}, nil
	}
	return instructions.Document{
		Key:       instructions.Key{OrgID: rec.OrgID, Scope: key.Scope, ScopeID: rec.ScopeID, Kind: key.Kind},
		Content:   rec.BodyMD,
		Version:   rec.Version,
		UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (s *InstructionsStore) Save(ctx context.Context, key instructions.Key, content, expectedVersion string) (instructions.Document, error) {
	if err := key.Validate(); err != nil {
		return instructions.Document{}, err
	}
	acc, ok := s.cloud.activeAccount(ctx)
	if !ok || key.Scope == instructions.ScopeBot || (key.Scope == instructions.ScopeUser && key.Kind == instructions.KindBots) {
		return s.fallback.Save(ctx, key, content, expectedVersion)
	}
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
	saved, err := s.cloud.SaveInstruction(ctx, string(key.Scope), scopeID, string(key.Kind), content, expectedVersion)
	if err != nil {
		var uerr *UpstreamError
		if errors.As(err, &uerr) && uerr.StatusCode == http.StatusConflict {
			cur, _, _ := s.cloud.store.GetCloudInstruction(ctx, orgID, string(key.Scope), scopeID, string(key.Kind))
			return instructions.Document{
				Key:       key,
				Content:   cur.BodyMD,
				Version:   cur.Version,
				UpdatedAt: cur.UpdatedAt,
			}, instructions.ErrConflict
		}
		return instructions.Document{}, err
	}
	return instructions.Document{
		Key:       instructions.Key{OrgID: saved.OrgID, Scope: key.Scope, ScopeID: saved.ScopeID, Kind: key.Kind},
		Content:   saved.BodyMD,
		Version:   saved.Version,
		UpdatedAt: saved.UpdatedAt,
	}, nil
}

func (s *Service) CloudScope(ctx context.Context) *coordinator.CloudScope {
	acc, ok := s.activeAccount(ctx)
	if !ok || acc.OrgID == "" || acc.UserID == "" {
		return nil
	}
	return &coordinator.CloudScope{OrgID: acc.OrgID, UserID: acc.UserID}
}

func (s *Service) MCPServer(ctx context.Context) (mcpconfig.Server, bool) {
	acc, ok := s.activeAccount(ctx)
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
