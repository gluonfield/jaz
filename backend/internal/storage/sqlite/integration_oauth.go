package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	oauthdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/integrationoauth"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func (s *Store) LoadToken(ctx context.Context, connectionID string) (integrationoauth.Token, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := oauthdb.New(s.db).LoadToken(ctx, connectionID)
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
	data, err := tokenJSON(token)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return oauthdb.New(s.db).SaveToken(ctx, oauthdb.SaveTokenParams{
		ConnectionID: connectionID,
		TokenJson:    data,
		UpdatedAtMs:  timeToMs(time.Now().UTC()),
	})
}

func tokenJSON(token integrationoauth.Token) (string, error) {
	data, err := json.Marshal(token)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Store) DeleteToken(ctx context.Context, connectionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return oauthdb.New(s.db).DeleteToken(ctx, connectionID)
}
