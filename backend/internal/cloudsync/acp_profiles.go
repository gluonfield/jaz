package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

var ErrWorkspaceChanged = errors.New("active Cloud workspace changed before sign-in completed")

// PutACPProfileForWorkspace uploads an encrypted personal ACP credential bundle
// strictly to the named Cloud workspace so an async sign-in started in one
// workspace can never upload tokens into a different workspace after a switch.
func (s *Service) PutACPProfileForWorkspace(ctx context.Context, workspaceID, agent string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(agent); err != nil {
		return nil, 0, err
	}
	s.mu.RLock()
	active := s.activeWorkspaceID
	acc, ok := s.accounts[workspaceID]
	s.mu.RUnlock()
	if !ok || acc.Token == "" || workspaceID == "" || workspaceID == LocalWorkspaceID {
		return nil, 0, ErrNotConnected
	}
	if active != workspaceID {
		return nil, 0, ErrWorkspaceChanged
	}
	raw, status, err := s.clientFor(acc.ID).doJSON(ctx, http.MethodPut, acc.BaseURL, "/v1/acp/profiles/"+url.PathEscape(agent), acc.Token, body)
	if err == nil && status < 300 {
		s.InvalidateModelsCache()
	}
	return raw, status, err
}

// InvalidateModelsCache drops the short-lived /v1/models response cache so
// newly connected or disconnected personal ACP profiles appear immediately.
func (s *Service) InvalidateModelsCache() {
	s.mu.Lock()
	delete(s.endpointCache, "/v1/models")
	s.mu.Unlock()
}
