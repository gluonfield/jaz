package app

import (
	"github.com/wins/jaz/backend/internal/runtimeauth"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type RuntimeAuthKey string

func NewRuntimeAuthKey(store *sqlitestore.Store) (RuntimeAuthKey, error) {
	key, err := runtimeauth.Ensure(store.RootDir())
	return RuntimeAuthKey(key), err
}
