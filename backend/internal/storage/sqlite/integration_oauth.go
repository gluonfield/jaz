package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func (s *Store) LoadToken(ctx context.Context, connectionID string) (integrationoauth.Token, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT token_json FROM integration_oauth_tokens WHERE connection_id = ? LIMIT 1`, connectionID).Scan(&data)
	if err == sql.ErrNoRows {
		return integrationoauth.Token{}, false, nil
	}
	if err != nil {
		return integrationoauth.Token{}, false, err
	}
	var token integrationoauth.Token
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return integrationoauth.Token{}, false, err
	}
	return token, true, nil
}

func (s *Store) SaveToken(ctx context.Context, connectionID string, token integrationoauth.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO integration_oauth_tokens (
  connection_id,
  token_json,
  updated_at_ms
) VALUES (?, ?, ?)
ON CONFLICT(connection_id) DO UPDATE SET
  token_json = excluded.token_json,
  updated_at_ms = excluded.updated_at_ms`, connectionID, string(data), timeToMs(time.Now().UTC()))
	return err
}

func (s *Store) DeleteToken(ctx context.Context, connectionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM integration_oauth_tokens WHERE connection_id = ?`, connectionID)
	return err
}
