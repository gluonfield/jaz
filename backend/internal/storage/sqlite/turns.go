package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) StartSessionTurn(id string, turn storage.Turn, messages ...storage.Message) error {
	if turn.StartedAt.IsZero() {
		turn.StartedAt = time.Now().UTC()
	}
	s.writeMu.Lock()
	err := func() error {
		tx, err := s.db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := threaddb.New(tx).StartSessionTurn(context.Background(), threaddb.StartSessionTurnParams{
			ID: id, Turn: sessionTurnJSON(&turn), StartedAtMs: timeToMs(turn.StartedAt),
		}); err != nil {
			return err
		}
		if err := appendMessageRecords(tx, id, messages, turn.StartedAt); err != nil {
			return err
		}
		return tx.Commit()
	}()
	s.writeMu.Unlock()
	if err == nil {
		if s.exportMirror != nil {
			if records, err := providerMessagesFromRecords(messages); err == nil {
				_ = s.exportMirror.AppendMessages(id, records...)
			}
		}
		if current, loadErr := s.LoadSession(id); loadErr == nil {
			s.mirrorSession(current)
		}
	}
	return err
}

// ClaimTurn starts turn on the thread unless another turn holds it, and
// reports whether it started it.
func (s *Store) ClaimTurn(id string, turn storage.Turn) (bool, error) {
	turn.StartedAt = time.Now().UTC()
	s.writeMu.Lock()
	started, err := threaddb.New(s.db).ClaimTurn(context.Background(), threaddb.ClaimTurnParams{
		ID: id, Turn: sessionTurnJSON(&turn), StartedAtMs: timeToMs(turn.StartedAt),
	})
	s.writeMu.Unlock()
	if err != nil || started == 0 {
		return false, err
	}
	if current, loadErr := s.LoadSession(id); loadErr == nil {
		s.mirrorSession(current)
	}
	return true, nil
}

// OweTurn has the thread's running turn, if any, also owe whom owed owes
// (see storage.Turn.Owing).
func (s *Store) OweTurn(id string, owed storage.Turn) error {
	ctx := context.Background()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := threaddb.New(tx)
	raw, err := q.GetRunningTurn(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	running, err := parseSessionTurn(raw)
	if err != nil {
		return err
	}
	turn := running.Owing(owed)
	if err := q.SetRunningTurn(ctx, threaddb.SetRunningTurnParams{ID: id, Turn: sessionTurnJSON(&turn)}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetTurnPromptState(id, agentSessionID string, startedAt time.Time, state storage.PromptState) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	updated, err := threaddb.New(s.db).SetTurnPromptState(context.Background(), threaddb.SetTurnPromptStateParams{ID: id, AgentSessionID: nullDBString(agentSessionID), StartedAt: startedAt.UTC().Format(time.RFC3339Nano), State: string(state)})
	if err == nil && updated == 0 {
		return fmt.Errorf("%s: turn or agent session changed before prompt state was recorded", id)
	}
	return err
}

// AppendTurnReply adds a reply to the running turn's answer, with the
// commitments it delivers.
func (s *Store) AppendTurnReply(id, message string, done []int64) error {
	if done == nil {
		done = []int64{}
	}
	ids, err := json.Marshal(done)
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
	for _, commitment := range done {
		if err := closable(ctx, botdb.New(tx), commitment); err != nil {
			return err
		}
	}
	updated, err := threaddb.New(tx).AppendTurnReply(ctx, threaddb.AppendTurnReplyParams{ID: id, Message: message, Done: string(ids)})
	if err != nil {
		return err
	}
	if updated == 0 {
		return fmt.Errorf("%s has no active reply turn", id)
	}
	return tx.Commit()
}

// ResetTurnHops ends the bot-to-bot exchange a running turn is part of, as
// the user writing into it does.
func (s *Store) ResetTurnHops(id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return threaddb.New(s.db).ResetTurnHops(context.Background(), id)
}

func (s *Store) SetTurnIntent(id string, goalRequested, parentVisible, notifyParent bool) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	updated, err := threaddb.New(s.db).SetTurnIntent(context.Background(), threaddb.SetTurnIntentParams{
		ID: id, GoalRequested: boolInt(goalRequested), ParentVisible: boolInt(parentVisible), NotifyParent: boolInt(notifyParent),
	})
	if err == nil && updated == 0 {
		return fmt.Errorf("%s has no active turn", id)
	}
	return err
}

func (s *Store) SetTurnRetries(id string, retries int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	updated, err := threaddb.New(s.db).SetTurnRetries(context.Background(), threaddb.SetTurnRetriesParams{ID: id, Retries: int64(retries)})
	if err == nil && updated == 0 {
		return fmt.Errorf("%s has no active turn", id)
	}
	return err
}

func (s *Store) ClaimQueuedTurn(session storage.Session, prompt storage.QueuedMessage) error {
	queue, err := storage.MarshalQueuedMessages(session.QueuedMessages)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	claimed, err := threaddb.New(s.db).ClaimQueuedTurn(context.Background(), threaddb.ClaimQueuedTurnParams{
		ID: session.ID, QueuedMessages: queue, Title: nullDBString(session.Title),
		Turn: sessionTurnJSON(prompt.Turn(now)), StartedAtMs: timeToMs(now),
	})
	if err == nil && claimed == 0 {
		return storage.ErrThreadRunning
	}
	return err
}

func (s *Store) FinishSessionTurn(id, status, errorMessage string, at time.Time, deliveries []storage.TurnDelivery) error {
	if err := s.finishSessionTurn(id, status, errorMessage, at, deliveries); err != nil {
		return err
	}
	ids := []string{id}
	for _, delivery := range deliveries {
		ids = append(ids, delivery.SessionID)
	}
	for _, id := range ids {
		if current, err := s.LoadSession(id); err == nil {
			s.mirrorSession(current)
		}
	}
	s.notifySessionEventCompaction()
	return nil
}

func (s *Store) finishSessionTurn(id, status, errorMessage string, at time.Time, deliveries []storage.TurnDelivery) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := threaddb.New(tx)
	updated, err := q.FinishSessionTurn(ctx, threaddb.FinishSessionTurnParams{
		ID: id, Status: status, Error: nullDBString(errorMessage), FinishedAtMs: timeToMs(at),
	})
	if err != nil || updated == 0 {
		return err
	}
	for _, delivery := range deliveries {
		if delivery.Wake {
			return fmt.Errorf("a finished turn's delivery cannot wake a bot")
		}
		queued, err := s.queueMessage(ctx, q, delivery.SessionID, delivery.Message)
		if err != nil {
			return err
		}
		if !queued && delivery.Fallback != nil {
			if _, err := s.queueMessage(ctx, q, id, *delivery.Fallback); err != nil {
				return err
			}
		}
		if !queued {
			continue
		}
		if err := closeAnswered(ctx, botdb.New(tx), delivery.Closes, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// QueueTurn queues a delivery for its thread's next turn and closes the
// commitments it delivers, or records the stalls it wakes its bot about,
// together or not at all.
func (s *Store) QueueTurn(delivery storage.TurnDelivery) error {
	if delivery.Fallback != nil {
		return fmt.Errorf("a queued turn has no fallback")
	}
	ctx := context.Background()
	s.writeMu.Lock()
	err := func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if delivery.Wake {
			woken, err := botdb.New(tx).WakeStalled(ctx, botdb.WakeStalledParams{BotID: delivery.SessionID, NowMs: time.Now().UnixMilli()})
			if err != nil {
				return err
			}
			if len(woken) == 0 {
				return storage.ErrNothingStalled
			}
		}
		queued, err := s.queueMessage(ctx, threaddb.New(tx), delivery.SessionID, delivery.Message)
		if err != nil {
			return err
		}
		if !queued {
			return fmt.Errorf("%s: %w", delivery.SessionID, storage.ErrSessionNotFound)
		}
		if err := closeDelivered(ctx, botdb.New(tx), delivery.Closes, time.Now()); err != nil {
			return err
		}
		return tx.Commit()
	}()
	s.writeMu.Unlock()
	if err == nil {
		if current, loadErr := s.LoadSession(delivery.SessionID); loadErr == nil {
			s.mirrorSession(current)
		}
	}
	return err
}

// queueMessage queues message in thread unless the thread is archived or
// gone.
func (s *Store) queueMessage(ctx context.Context, q *threaddb.Queries, thread string, message storage.QueuedMessage) (bool, error) {
	message.ID = s.NewSessionID()
	raw, err := storage.MarshalQueuedMessage(&message)
	if err != nil {
		return false, err
	}
	updated, err := q.AppendQueuedTurn(ctx, threaddb.AppendQueuedTurnParams{ID: thread, Message: raw})
	return updated > 0, err
}

func sessionTurnJSON(turn *storage.Turn) string {
	if turn == nil {
		return ""
	}
	data, _ := json.Marshal(turn)
	return string(data)
}

func parseSessionTurn(raw string) (*storage.Turn, error) {
	var turn *storage.Turn
	if raw == "" {
		return nil, nil
	}
	err := json.Unmarshal([]byte(raw), &turn)
	return turn, err
}
