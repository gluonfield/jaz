package app

import (
	"github.com/wins/jaz/backend/internal/connections"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func NewConnectionOAuthService(store *sqlitestore.Store) *connections.OAuthService {
	return connections.NewOAuthService(store)
}

func NewConnectionService(catalog *connections.Catalog, store *sqlitestore.Store) *connections.Service {
	return connections.NewService(catalog, store)
}
