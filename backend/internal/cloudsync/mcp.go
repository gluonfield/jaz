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
	sourceID    string
}

type nativeMCPContext struct {
	mu       sync.Mutex
	threadID string
}

func (s *Service) CallMCP(ctx context.Context, session storage.Session, body json.RawMessage, bindThread bool) (json.RawMessage, int, error) {
	acc, ok := s.currentAccountToken(session.WorkspaceID)
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	client := s.clientFor(acc.ID)
	if bindThread {
		threadID, err := s.nativeMCPThread(ctx, acc, session.ID)
		if err != nil {
			return nil, 0, err
		}
		client.mcpThreadID = threadID
	}
	acc, ok = s.currentAccountToken(acc.ID)
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	return client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/mcp", acc.Token, body)
}

func (s *Service) nativeMCPThread(ctx context.Context, acc storage.CloudAccount, sessionID string) (string, error) {
	// The broker requires an authorized Cloud thread. The opaque source ID lets
	// the desktop recover its private thread after restart without syncing local chat.
	key := nativeMCPKey{acc.ID, fmt.Sprintf("%x", sha256.Sum256([]byte(sessionID)))}
	actual, _ := s.mcpThreads.LoadOrStore(key, &nativeMCPContext{})
	binding := actual.(*nativeMCPContext)
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.threadID != "" {
		return binding.threadID, nil
	}
	var err error
	binding.threadID, err = s.findOrCreateMCPThread(ctx, acc, key.sourceID)
	return binding.threadID, err
}

func (s *Service) findOrCreateMCPThread(ctx context.Context, acc storage.CloudAccount, sourceID string) (string, error) {
	client := s.clientFor(acc.ID)
	threads, err := client.listThreads(ctx, acc.BaseURL, acc.Token)
	if err != nil {
		return "", err
	}
	for _, thread := range threads {
		if thread.SourceType == nativeMCPSource && thread.SourceID == sourceID &&
			thread.Visibility == "private" && thread.OwnerUserID == acc.UserID && thread.OrgID == acc.OrgID {
			return thread.ID, nil
		}
	}
	thread, err := client.createThread(ctx, acc.BaseURL, s.latestToken(acc), CreateThreadRequest{
		SourceType: nativeMCPSource,
		SourceID:   sourceID,
	})
	if err != nil {
		return "", err
	}
	if thread.ID == "" || thread.Visibility != "private" || thread.OwnerUserID != acc.UserID || thread.OrgID != acc.OrgID {
		return "", errors.New("cloud returned an invalid native MCP context")
	}
	return thread.ID, nil
}
