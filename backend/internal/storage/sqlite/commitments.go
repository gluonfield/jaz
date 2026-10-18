package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
)

func (s *Store) OpenCommitment(commitment storage.Commitment) (storage.Commitment, error) {
	commitment.Status = storage.CommitmentOpen
	commitment.OpenedAt = time.Now().UTC()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	id, err := botdb.New(s.db).InsertCommitment(context.Background(), botdb.InsertCommitmentParams{
		BotID: commitment.BotID, Requester: commitment.Requester, Request: commitment.Request, Note: commitment.Note, OpenedAtMs: timeToMs(commitment.OpenedAt),
	})
	commitment.ID = id
	return commitment, err
}

// UpdateCommitment saves an open commitment's request and note.
func (s *Store) UpdateCommitment(commitment storage.Commitment) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	updated, err := botdb.New(s.db).UpdateCommitment(context.Background(), botdb.UpdateCommitmentParams{
		ID: commitment.ID, BotID: commitment.BotID, Request: commitment.Request, Note: commitment.Note,
	})
	if err == nil && updated == 0 {
		return storage.ErrCommitmentNotOpen
	}
	return err
}

// DropCommitment closes an open commitment without a delivery, saying why.
func (s *Store) DropCommitment(id int64, why string, at time.Time) error {
	ctx := context.Background()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := closeCommitment(ctx, botdb.New(tx), id, storage.CommitmentDropped, why, at); err != nil {
		return err
	}
	return tx.Commit()
}

// closeDelivered closes as done the commitments a delivery written in the
// same transaction delivers.
func closeDelivered(ctx context.Context, q *botdb.Queries, ids []int64, at time.Time) error {
	for _, id := range ids {
		if err := closeCommitment(ctx, q, id, storage.CommitmentDone, "", at); err != nil {
			return err
		}
	}
	return nil
}

// closeCommitment closes an open commitment none of whose threads works.
func closeCommitment(ctx context.Context, q *botdb.Queries, id int64, status, note string, at time.Time) error {
	working, err := q.CommitmentWorking(ctx, id)
	if err != nil {
		return err
	}
	if working {
		return storage.ErrCommitmentWorking
	}
	closed, err := q.CloseCommitment(ctx, botdb.CloseCommitmentParams{
		ID: id, Status: status, Note: nullDBString(note), ClosedAtMs: sql.NullInt64{Int64: timeToMs(at), Valid: true},
	})
	if err == nil && closed == 0 {
		return storage.ErrCommitmentNotOpen
	}
	return err
}

func (s *Store) LoadCommitment(id int64) (storage.Commitment, error) {
	ctx := context.Background()
	q := botdb.New(s.db)
	row, err := q.GetCommitment(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Commitment{}, storage.ErrCommitmentNotOpen
	}
	if err != nil {
		return storage.Commitment{}, err
	}
	commitments, err := withThreads(ctx, q, []botdb.BotCommitment{row})
	if err != nil {
		return storage.Commitment{}, err
	}
	return commitments[0], nil
}

// ListOpenCommitments returns what a bot owes, oldest first.
func (s *Store) ListOpenCommitments(botID string) ([]storage.Commitment, error) {
	ctx := context.Background()
	q := botdb.New(s.db)
	rows, err := q.ListOpenCommitments(ctx, botID)
	if err != nil {
		return nil, err
	}
	return withThreads(ctx, q, rows)
}

// ListClosedCommitments returns a bot's commitments closed from since on.
func (s *Store) ListClosedCommitments(botID string, since time.Time) ([]storage.Commitment, error) {
	ctx := context.Background()
	q := botdb.New(s.db)
	rows, err := q.ListClosedCommitments(ctx, botdb.ListClosedCommitmentsParams{BotID: botID, SinceMs: sql.NullInt64{Int64: timeToMs(since), Valid: true}})
	if err != nil {
		return nil, err
	}
	return withThreads(ctx, q, rows)
}

func withThreads(ctx context.Context, q *botdb.Queries, rows []botdb.BotCommitment) ([]storage.Commitment, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	links, err := q.ListCommitmentThreads(ctx, ids)
	if err != nil {
		return nil, err
	}
	threads := map[int64][]string{}
	for _, link := range links {
		threads[link.CommitmentID] = append(threads[link.CommitmentID], link.ThreadID)
	}
	commitments := make([]storage.Commitment, 0, len(rows))
	for _, row := range rows {
		commitments = append(commitments, storage.Commitment{
			ID: row.ID, BotID: row.BotID, Requester: row.Requester, Request: row.Request, Note: row.Note, Status: row.Status,
			OpenedAt: msToTime(row.OpenedAtMs), ClosedAt: msToTime(row.ClosedAtMs.Int64), Threads: threads[row.ID],
		})
	}
	return commitments, nil
}
