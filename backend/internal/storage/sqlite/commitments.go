package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
)

func (s *Store) OpenCommitment(commitment storage.Commitment) (storage.Commitment, error) {
	commitment.Status = storage.CommitmentPending
	commitment.OpenedAt = time.Now().UTC()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	id, err := botdb.New(s.db).InsertCommitment(context.Background(), botdb.InsertCommitmentParams{
		BotID: commitment.BotID, Requester: commitment.Requester, Request: commitment.Request, Note: commitment.Note, OpenedAtMs: timeToMs(commitment.OpenedAt),
	})
	commitment.ID = id
	return commitment, err
}

func (s *Store) UpdateCommitment(commitment storage.Commitment) error {
	ctx := context.Background()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updateCommitment(ctx, botdb.New(tx), commitment); err != nil {
		return err
	}
	return tx.Commit()
}

func updateCommitment(ctx context.Context, q *botdb.Queries, commitment storage.Commitment) error {
	if !commitment.IsOpen() {
		return fmt.Errorf("invalid open task status %q", commitment.Status)
	}
	if (commitment.Status == storage.CommitmentWaitingForUser) != (commitment.QuestionSeq > 0) {
		return errors.New("waiting_for_user requires a delivered question")
	}
	updated, err := q.UpdateCommitment(ctx, botdb.UpdateCommitmentParams{
		ID: commitment.ID, BotID: commitment.BotID, Request: commitment.Request, Note: commitment.Note,
		Status: commitment.Status, QuestionSeq: commitment.QuestionSeq, QuestionUserSeq: commitment.QuestionUserSeq,
	})
	if err != nil {
		return err
	}
	if updated == 0 {
		return fmt.Errorf("#%d: %w", commitment.ID, storage.ErrCommitmentNotOpen)
	}
	if err := q.DeleteCommitmentDependencies(ctx, commitment.ID); err != nil {
		return err
	}
	for _, prerequisite := range commitment.DependsOn {
		cycle, err := q.CommitmentDependencyCycle(ctx, botdb.CommitmentDependencyCycleParams{CommitmentID: commitment.ID, PrerequisiteID: prerequisite})
		if err != nil {
			return err
		}
		if cycle {
			return errors.New("task dependencies must have no cycles")
		}
		linked, err := q.LinkCommitmentDependency(ctx, botdb.LinkCommitmentDependencyParams{CommitmentID: commitment.ID, PrerequisiteID: prerequisite})
		if err != nil {
			return err
		}
		if linked == 0 {
			return fmt.Errorf("invalid prerequisite #%d: tasks must belong to the same bot and dependencies must have no cycles", prerequisite)
		}
	}
	motions, err := q.ListCommitmentMotion(ctx, []int64{commitment.ID})
	if err != nil {
		return err
	}
	for _, motion := range motions {
		if motion.Moving != 0 && motion.Blocked != 0 {
			return errors.New("stop the task's active workers before adding unfinished prerequisites")
		}
	}
	return nil
}

func (s *Store) ListWatchBots() ([]string, error) {
	return botdb.New(s.db).ListWatchBots(context.Background(), time.Now().UnixMilli())
}

// DropCommitment closes an open commitment without a delivery, saying why;
// not one that the answer of its bot's running turn delivers.
func (s *Store) DropCommitment(id int64, why string, at time.Time) error {
	ctx := context.Background()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := botdb.New(tx)
	staged, err := q.CommitmentStaged(ctx, id)
	if err != nil {
		return err
	}
	if staged {
		return fmt.Errorf("#%d: %w", id, storage.ErrCommitmentStaged)
	}
	if err := closeCommitment(ctx, q, id, storage.CommitmentDropped, why, at); err != nil {
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

// closeAnswered closes as done the commitments a finished turn's answer
// delivers. The answer was refused up front for any that could not close, so
// one that cannot close now was taken up again later in the turn and stays
// open.
func closeAnswered(ctx context.Context, q *botdb.Queries, ids []int64, at time.Time) error {
	for _, id := range ids {
		err := closeCommitment(ctx, q, id, storage.CommitmentDone, "", at)
		if err != nil && !errors.Is(err, storage.ErrCommitmentNotOpen) && !errors.Is(err, storage.ErrCommitmentWorking) {
			return err
		}
	}
	return nil
}

// closable refuses a commitment that is closed or one of whose threads still
// works.
func closable(ctx context.Context, q *botdb.Queries, id int64) error {
	row, err := q.GetCommitment(ctx, id)
	commitment := storage.Commitment{Status: row.Status}
	if errors.Is(err, sql.ErrNoRows) || err == nil && !commitment.IsOpen() {
		return fmt.Errorf("#%d: %w", id, storage.ErrCommitmentNotOpen)
	}
	if err != nil {
		return err
	}
	working, err := q.CommitmentWorking(ctx, id)
	if err == nil && working {
		err = fmt.Errorf("#%d: %w", id, storage.ErrCommitmentWorking)
	}
	return err
}

// closeCommitment closes an open commitment none of whose threads works. One
// closed as done delivered the results its threads queued for its bot.
func closeCommitment(ctx context.Context, q *botdb.Queries, id int64, status, note string, at time.Time) error {
	if err := closable(ctx, q, id); err != nil {
		return err
	}
	if _, err := q.CloseCommitment(ctx, botdb.CloseCommitmentParams{
		ID: id, Status: status, Note: nullDBString(note), ClosedAtMs: sql.NullInt64{Int64: timeToMs(at), Valid: true},
	}); err != nil || status != storage.CommitmentDone {
		return err
	}
	return q.DropDeliveredResults(ctx, id)
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
	motions, err := q.ListCommitmentMotion(ctx, ids)
	if err != nil {
		return nil, err
	}
	motion := map[int64]botdb.ListCommitmentMotionRow{}
	for _, row := range motions {
		motion[row.ID] = row
	}
	prerequisites, err := q.ListCommitmentDependencies(ctx, ids)
	if err != nil {
		return nil, err
	}
	dependencies := map[int64][]int64{}
	for _, row := range prerequisites {
		dependencies[row.CommitmentID] = append(dependencies[row.CommitmentID], row.PrerequisiteID)
	}
	threads := map[int64][]storage.CommitmentThread{}
	for _, link := range links {
		threads[link.CommitmentID] = append(threads[link.CommitmentID], storage.CommitmentThread{ID: link.ThreadID, Slug: link.Slug, Title: link.Title.String, State: link.State})
	}
	commitments := make([]storage.Commitment, 0, len(rows))
	for _, row := range rows {
		commitments = append(commitments, storage.Commitment{
			ID: row.ID, BotID: row.BotID, Requester: row.Requester, Request: row.Request, Note: row.Note, QuestionSeq: row.QuestionSeq, QuestionUserSeq: row.QuestionUserSeq, DependsOn: dependencies[row.ID],
			Moving: motion[row.ID].Moving != 0, Blocked: motion[row.ID].Blocked != 0, Stalled: motion[row.ID].Stalled != 0, Status: row.Status,
			OpenedAt: msToTime(row.OpenedAtMs), ClosedAt: msToTime(row.ClosedAtMs.Int64), Threads: threads[row.ID],
		})
	}
	return commitments, nil
}
