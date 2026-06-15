package sqlite

import "github.com/wins/jaz/backend/internal/storage/sqlite/generated/search"

func NewSearchQueries(store *Store) search.Querier {
	return store.searchQueries
}
