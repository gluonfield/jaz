package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
)

func (s *Store) ArchiveFinishedBotWork(ctx context.Context, before time.Time, worktreeRoot string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := botdb.New(tx)
	ids, err := q.ListFinishedBotWork(ctx, sql.NullInt64{Int64: timeToMs(before), Valid: true})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.ArchiveFinishedBotWork(ctx, botdb.ArchiveFinishedBotWorkParams{ID: id, BeforeMs: timeToMs(before), WorktreeRoot: worktreeRoot}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
