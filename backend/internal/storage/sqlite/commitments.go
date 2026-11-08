package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) OpenCommitment(commitment storage.Commitment) (storage.Commitment, error) {
	commitment.Status = storage.CommitmentPending
	commitment.OpenedAt = time.Now().UTC()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	id, err := botdb.New(s.db).InsertCommitment(context.Background(), botdb.InsertCommitmentParams{
		BotID: commitment.BotID, Requester: commitment.Requester, Name: commitment.Name, Request: commitment.Request, Note: commitment.Note, OpenedAtMs: timeToMs(commitment.OpenedAt),
		OpenedTurnMs: loopTimeToMs(commitment.OpenedTurn),
	})
	commitment.ID = id
	return commitment, err
}

// OpenedTask reports whether a bot opened any task in its turn that began at
// turn.
func (s *Store) OpenedTask(botID string, turn time.Time) (bool, error) {
	return botdb.New(s.db).OpenedTask(context.Background(), botdb.OpenedTaskParams{BotID: botID, TurnMs: loopTimeToMs(turn)})
}

// Empty text/status and nil dependencies preserve their current values.
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
	if commitment.Status == storage.CommitmentStopped {
		return storage.ErrTaskStopped
	}
	if commitment.Status != "" && !commitment.IsOpen() {
		return fmt.Errorf("invalid open task status %q", commitment.Status)
	}
	if commitment.Status == storage.CommitmentWaitingForUser && commitment.QuestionSeq <= 0 {
		return errors.New("waiting_for_user requires a delivered question")
	}
	checkAt := sql.NullInt64{}
	if commitment.CheckAt != nil {
		checkAt.Valid = true
		if !commitment.CheckAt.IsZero() {
			checkAt.Int64 = commitment.CheckAt.UnixMilli()
		}
	}
	updated, err := q.UpdateCommitment(ctx, botdb.UpdateCommitmentParams{
		ID: commitment.ID, BotID: commitment.BotID, Name: nullDBString(commitment.Name), Request: nullDBString(commitment.Request), Note: nullDBString(commitment.Note),
		Status: nullDBString(commitment.Status), QuestionSeq: commitment.QuestionSeq, CheckAtMs: checkAt,
	})
	if err != nil {
		return err
	}
	if updated == 0 {
		return fmt.Errorf("#%d: %w", commitment.ID, storage.ErrCommitmentNotOpen)
	}
	if commitment.DependsOn != nil {
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
	}
	motions, err := q.ListCommitmentMotion(ctx, []int64{commitment.ID})
	if err != nil {
		return err
	}
	for _, motion := range motions {
		if motion.Moving != 0 && (motion.Blocked != 0 || commitment.Status == storage.CommitmentPending) {
			return errors.New("an active task cannot be pending or have unfinished prerequisites")
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
	row, err := q.GetCommitment(ctx, id)
	if err != nil {
		return err
	}
	if row.Status == storage.CommitmentStopped {
		if err := retireCommitmentQueues(ctx, tx, id, row.BotID, true); err != nil {
			return err
		}
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
	if row.Status == storage.CommitmentStopped {
		return storage.ErrTaskStopped
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
	if err := closable(ctx, q, id); err != nil && !(status == storage.CommitmentDropped && errors.Is(err, storage.ErrTaskStopped)) {
		return err
	}
	if _, err := q.CloseCommitment(ctx, botdb.CloseCommitmentParams{
		ID: id, Status: status, Note: nullDBString(note), ClosedAtMs: sql.NullInt64{Int64: timeToMs(at), Valid: true},
	}); err != nil || status != storage.CommitmentDone {
		return err
	}
	return q.DropDeliveredResults(ctx, id)
}

// LinkCommitmentThread has a thread the bot started, which may already serve
// other tasks, serve its open commitment too. Retention may have archived it
// as finished work; serving an open commitment, it is work again.
func (s *Store) LinkCommitmentThread(bot, thread string, commitment int64) error {
	session, err := s.LoadSession(thread)
	if err != nil {
		return err
	}
	ctx := context.Background()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := botdb.New(tx)
	if err := taskAdmission(ctx, q, session.ID); err != nil {
		return err
	}
	if err := linkCommitmentThread(ctx, q, bot, session.ID, commitment); err != nil {
		return err
	}
	if err := threaddb.New(tx).SetArchived(ctx, threaddb.SetArchivedParams{Archived: 0, Ids: []string{session.ID}}); err != nil {
		return err
	}
	return tx.Commit()
}

func linkCommitmentThread(ctx context.Context, q *botdb.Queries, bot, thread string, commitment int64) error {
	linked, err := q.LinkCommitmentThread(ctx, botdb.LinkCommitmentThreadParams{ThreadID: thread, CommitmentID: commitment, BotID: bot})
	if err != nil {
		return err
	}
	if linked == 0 {
		return fmt.Errorf("#%d: %w for a thread this bot started", commitment, storage.ErrCommitmentNotOpen)
	}
	return q.StartCommitment(ctx, commitment)
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
	commitments, err := s.withThreads(ctx, q, []botdb.BotCommitment{row})
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
	return s.withThreads(ctx, q, rows)
}

func (s *Store) withThreads(ctx context.Context, q *botdb.Queries, rows []botdb.BotCommitment) ([]storage.Commitment, error) {
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
	lastActivity := s.threadLastActivity(ctx, q, links)
	threads := map[int64][]storage.CommitmentThread{}
	for _, link := range links {
		threads[link.CommitmentID] = append(threads[link.CommitmentID], storage.CommitmentThread{ID: link.ThreadID, Slug: link.Slug, Title: link.Title.String, State: link.State, LastActivity: lastActivity[link.ThreadID]})
	}
	questions, err := q.ListCommitmentQuestions(ctx, ids)
	if err != nil {
		return nil, err
	}
	question := map[int64]string{}
	for _, row := range questions {
		question[row.ID] = row.Question
	}
	opening, err := q.ListCommitmentOpeningMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	openingSeq := map[int64]int64{}
	for _, row := range opening {
		at, err := time.Parse(time.RFC3339Nano, row.Answers)
		if err == nil && at.UnixMilli() == row.OpenedTurnMs && openingSeq[row.ID] == 0 {
			openingSeq[row.ID] = row.Seq
		}
	}
	commitments := make([]storage.Commitment, 0, len(rows))
	for _, row := range rows {
		var checkAt *time.Time
		if row.CheckAtMs > 0 {
			at := msToTime(row.CheckAtMs)
			checkAt = &at
		}
		commitments = append(commitments, storage.Commitment{
			ID: row.ID, BotID: row.BotID, Requester: row.Requester, Name: row.Name, Request: row.Request, Note: row.Note, QuestionSeq: row.QuestionSeq, Question: question[row.ID], OpeningSeq: openingSeq[row.ID], ReplySeq: row.ReplySeq, CheckAt: checkAt, DependsOn: dependencies[row.ID],
			Moving: motion[row.ID].Moving != 0, Blocked: motion[row.ID].Blocked != 0, Stalled: motion[row.ID].Stalled != 0, Status: row.Status,
			OpenedAt: msToTime(row.OpenedAtMs), OpenedTurn: msToTime(row.OpenedTurnMs), ClosedAt: msToTime(row.ClosedAtMs.Int64), Threads: threads[row.ID],
		})
	}
	return commitments, nil
}

// threadLastActivity maps each thread to its newest event time, so task
// views can show how recently a worker actually did anything.
func (s *Store) threadLastActivity(ctx context.Context, q *botdb.Queries, links []botdb.ListCommitmentThreadsRow) map[string]time.Time {
	out := map[string]time.Time{}
	ids := make([]string, 0, len(links))
	seen := map[string]bool{}
	for _, link := range links {
		if !seen[link.ThreadID] {
			seen[link.ThreadID] = true
			ids = append(ids, link.ThreadID)
		}
	}
	if len(ids) == 0 {
		return out
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := "SELECT thread_id, MAX(created_at_ms) FROM session_events WHERE thread_id IN (" + strings.Join(placeholders, ",") + ") GROUP BY thread_id"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var threadID string
		var atMs int64
		if err := rows.Scan(&threadID, &atMs); err != nil {
			continue
		}
		if atMs > 0 {
			out[threadID] = time.UnixMilli(atMs)
		}
	}
	return out
}
