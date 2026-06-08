package sqlite

import (
	"database/sql"
	"encoding/json"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

func (s *Store) LoadMCPOAuthToken(serverID string) (mcpconfig.OAuthToken, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var data string
	err := s.db.QueryRow(`SELECT token_json FROM mcp_oauth_tokens WHERE server_id = ?`, serverID).Scan(&data)
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
	_, err = s.db.Exec(`INSERT INTO mcp_oauth_tokens (server_id, token_json, updated_at_ms)
VALUES (?, ?, ?)
ON CONFLICT(server_id) DO UPDATE SET token_json = excluded.token_json, updated_at_ms = excluded.updated_at_ms`,
		serverID, string(data), timeToMs(time.Now().UTC()))
	return err
}

func (s *Store) DeleteMCPOAuthToken(serverID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM mcp_oauth_tokens WHERE server_id = ?`, serverID)
	return err
}
