package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	connectiondb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/integrationconnections"
	oauthdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/integrationoauth"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func (s *Store) LoadConnection(ctx context.Context, id string) (integrations.Connection, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := connectiondb.New(s.db).LoadConnection(ctx, id)
	if err == sql.ErrNoRows {
		return integrations.Connection{}, false, nil
	}
	if err != nil {
		return integrations.Connection{}, false, err
	}
	connection, err := connectionFromRow(row.ID, row.Provider, row.AccountID, row.AccountName, row.Alias, row.ScopesJson)
	return connection, true, err
}

func (s *Store) ListConnections(ctx context.Context, provider string) ([]integrations.Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := connectiondb.New(s.db).ListConnectionsByProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	out := make([]integrations.Connection, 0, len(rows))
	for _, row := range rows {
		connection, err := connectionFromRow(row.ID, row.Provider, row.AccountID, row.AccountName, row.Alias, row.ScopesJson)
		if err != nil {
			return nil, err
		}
		out = append(out, connection)
	}
	return out, nil
}

func (s *Store) SaveConnection(ctx context.Context, connection integrations.Connection) error {
	params, err := saveConnectionParams(connection, time.Now().UTC())
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return connectiondb.New(s.db).SaveConnection(ctx, params)
}

func (s *Store) SaveOAuthConnection(ctx context.Context, token integrationoauth.Token, connection integrations.Connection) error {
	tokenData, err := tokenJSON(token)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	connectionParams, err := saveConnectionParams(connection, now)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := oauthdb.New(s.db).WithTx(tx).SaveToken(ctx, oauthdb.SaveTokenParams{
		ConnectionID: connection.ID,
		TokenJson:    tokenData,
		UpdatedAtMs:  timeToMs(now),
	}); err != nil {
		return err
	}
	if err := connectiondb.New(s.db).WithTx(tx).SaveConnection(ctx, connectionParams); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteConnection(ctx context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	rows, err := connectiondb.New(s.db).WithTx(tx).DeleteConnection(ctx, id)
	if err != nil {
		return false, err
	}
	if rows == 0 {
		return false, nil
	}
	if err := oauthdb.New(s.db).WithTx(tx).DeleteToken(ctx, id); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func connectionFromRow(id, provider, accountID, accountName, alias, scopesJSON string) (integrations.Connection, error) {
	var scopes []string
	if err := json.Unmarshal([]byte(scopesJSON), &scopes); err != nil {
		return integrations.Connection{}, err
	}
	return integrations.Connection{
		ID:          id,
		Provider:    provider,
		AccountID:   accountID,
		AccountName: accountName,
		Alias:       alias,
		Scopes:      scopes,
	}, nil
}

func saveConnectionParams(connection integrations.Connection, now time.Time) (connectiondb.SaveConnectionParams, error) {
	scopes, err := json.Marshal(connection.Scopes)
	if err != nil {
		return connectiondb.SaveConnectionParams{}, err
	}
	return connectiondb.SaveConnectionParams{
		ID:          connection.ID,
		Provider:    connection.Provider,
		AccountID:   connection.AccountID,
		AccountName: connection.AccountName,
		Alias:       connection.Alias,
		ScopesJson:  string(scopes),
		UpdatedAtMs: timeToMs(now),
	}, nil
}
