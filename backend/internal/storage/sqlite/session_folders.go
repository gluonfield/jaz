package sqlite

import (
	"context"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) ListSessionFolders() ([]storage.SessionFolder, error) {
	rows, err := threaddb.New(s.db).ListSessionFolders(context.Background())
	if err != nil {
		return nil, err
	}
	folders := make([]storage.SessionFolder, 0, len(rows))
	for _, row := range rows {
		folders = append(folders, storage.SessionFolder{ID: row.ID, Title: row.Title.String, Status: row.Status, UpdatedAt: msToTime(row.UpdatedAtMs), Folder: row.Cwd.String})
	}
	return folders, nil
}
