package cloudsync

import (
	"context"
	"encoding/json"
	"net/http"
)

// PutACPProfile uploads an encrypted personal ACP credential bundle to the
// active Cloud workspace and invalidates the cached /v1/models catalog.
func (s *Service) PutACPProfile(ctx context.Context, agent string, body json.RawMessage) (json.RawMessage, int, error) {
	raw, status, err := s.authedSegmentJSON(ctx, http.MethodPut, "/v1/acp/profiles/", agent, "", body)
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
