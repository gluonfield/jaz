package sqlite

import "github.com/wins/jaz/backend/internal/storage/sqlite/generated/searchdb"

func NewSearchQueries(store *Store) searchdb.Querier {
	return store.searchQueries
}
