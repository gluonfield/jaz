package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/mcpdb"
)

func (s *Store) LoadMCPOAuthToken(serverID string) (mcpconfig.OAuthToken, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := mcpdb.New(s.db).GetMCPOAuthToken(context.Background(), serverID)
	if err == sql.ErrNoRows {
		return mcpconfig.OAuthToken{}, false, nil
	}
	if err != nil {
		return mcpconfig.OAuthToken{}, false, err
	}
	var token mcpconfig.OAuthToken
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return mcpconfig.OAuthToken{}, false, err
	}
	return token, true, nil
}

func (s *Store) SaveMCPOAuthToken(serverID string, token mcpconfig.OAuthToken) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return mcpdb.New(s.db).UpsertMCPOAuthToken(context.Background(), mcpdb.UpsertMCPOAuthTokenParams{
		ServerID:    serverID,
		TokenJson:   string(data),
		UpdatedAtMs: timeToMs(time.Now().UTC()),
	})
}

func (s *Store) DeleteMCPOAuthToken(serverID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return mcpdb.New(s.db).DeleteMCPOAuthToken(context.Background(), serverID)
}
