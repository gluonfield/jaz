package cloudsync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/wins/jaz/backend/internal/storage"
)

const nativeMCPSource = "native_mcp_context"

type nativeMCPKey struct {
	workspaceID string
	sessionID   string
}

type nativeMCPContext struct {
	mu       sync.Mutex
	threadID string
}

func (s *Service) CallMCP(ctx context.Context, session storage.Session, body json.RawMessage, bindThread bool) (json.RawMessage, int, error) {
	key := nativeMCPKey{session.WorkspaceID, session.ID}
	var binding *nativeMCPContext
	var headers http.Header
	if bindThread {
		actual, _ := s.mcpThreads.LoadOrStore(key, &nativeMCPContext{})
		binding = actual.(*nativeMCPContext)
		threadID, err := s.nativeMCPThread(ctx, key, binding)
		if err != nil {
			return nil, 0, err
		}
		headers = make(http.Header, 1)
		headers.Set("X-Jaz-Thread-ID", threadID)
	}
	acc, ok := s.currentAccountToken(session.WorkspaceID)
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	raw, status, err := s.clientFor(acc.ID).doJSONWithHeaders(ctx, http.MethodPost, acc.BaseURL, "/v1/mcp", acc.Token, body, headers)
	if binding != nil && mcpScopeDenied(raw) {
		// Resolve again on the next call. The current tool request is never replayed.
		s.mcpThreads.CompareAndDelete(key, binding)
	}
	return raw, status, err
}

func mcpScopeDenied(raw json.RawMessage) bool {
	var response struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	return json.Unmarshal(raw, &response) == nil && response.Error.Code == -32003
}

func (s *Service) nativeMCPThread(ctx context.Context, key nativeMCPKey, binding *nativeMCPContext) (string, error) {
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.threadID != "" {
		return binding.threadID, nil
	}
	acc, ok := s.currentAccountToken(key.workspaceID)
	if !ok || acc.Token == "" {
		return "", ErrNotConnected
	}
	// The broker requires an authorized Cloud thread. An opaque source ID
	// recovers that thread after a restart without sending the local transcript.
	sourceID := fmt.Sprintf("%x", sha256.Sum256([]byte(key.sessionID)))
	var err error
	binding.threadID, err = s.findOrCreateMCPThread(ctx, acc, sourceID)
	return binding.threadID, err
}

func (s *Service) findOrCreateMCPThread(ctx context.Context, acc storage.CloudAccount, sourceID string) (string, error) {
	matches := func(thread storage.CloudThread) bool {
		return thread.ID != "" && thread.SourceType == nativeMCPSource && thread.SourceID == sourceID &&
			thread.Visibility == "private" && thread.OwnerUserID == acc.UserID && thread.OrgID == acc.OrgID
	}
	client := s.clientFor(acc.ID)
	threads, err := client.listThreads(ctx, acc.BaseURL, acc.Token)
	if err != nil {
		return "", err
	}
	for _, thread := range threads {
		if matches(thread) {
			return thread.ID, nil
		}
	}
	acc, ok := s.currentAccountToken(acc.ID)
	if !ok || acc.Token == "" {
		return "", ErrNotConnected
	}
	thread, err := client.createThread(ctx, acc.BaseURL, acc.Token, CreateThreadRequest{
		SourceType: nativeMCPSource,
		SourceID:   sourceID,
	})
	if err != nil {
		return "", err
	}
	if !matches(thread) {
		return "", errors.New("cloud returned an invalid native MCP context")
	}
	return thread.ID, nil
}
