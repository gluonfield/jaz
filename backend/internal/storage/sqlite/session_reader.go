package sqlite

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

func ReadWorktreeSessions(root string) ([]storage.Session, error) {
	path, err := filepath.Abs(filepath.Join(root, "jaz.sqlite"))
	if err != nil {
		return nil, err
	}
	dsn := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/"), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	store := &Store{db: db}
	var all []storage.Session
	for _, archived := range []bool{false, true} {
		sessions, err := store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true, Archived: archived})
		if err != nil {
			return nil, err
		}
		all = append(all, sessions...)
	}
	return all, nil
}
