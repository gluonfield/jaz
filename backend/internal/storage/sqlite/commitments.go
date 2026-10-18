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

// CloseCommitment closes an open commitment as status; an empty note keeps
// the one it has.
func (s *Store) CloseCommitment(id int64, status, note string, at time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return closeCommitment(context.Background(), botdb.New(s.db), id, status, note, at)
}

func closeCommitment(ctx context.Context, q *botdb.Queries, id int64, status, note string, at time.Time) error {
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

// ListCommitments returns a bot's open commitments and those it closed from
// closedSince on, oldest first.
func (s *Store) ListCommitments(botID string, closedSince time.Time) ([]storage.Commitment, error) {
	ctx := context.Background()
	q := botdb.New(s.db)
	rows, err := q.ListBotCommitments(ctx, botdb.ListBotCommitmentsParams{BotID: botID, ClosedSinceMs: sql.NullInt64{Int64: timeToMs(closedSince), Valid: true}})
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
