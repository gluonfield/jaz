package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) StartSessionTurn(id string, turn storage.Turn) error {
	turn.StartedAt = time.Now().UTC()
	s.writeMu.Lock()
	err := threaddb.New(s.db).StartSessionTurn(context.Background(), threaddb.StartSessionTurnParams{
		ID: id, Turn: sessionTurnJSON(&turn), StartedAtMs: timeToMs(turn.StartedAt),
	})
	s.writeMu.Unlock()
	if err == nil {
		if current, loadErr := s.LoadSession(id); loadErr == nil {
			s.mirrorSession(current)
		}
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	updated, err := threaddb.New(s.db).AppendTurnReply(context.Background(), threaddb.AppendTurnReplyParams{ID: id, Message: message, Done: string(ids)})
	if err == nil && updated == 0 {
		return fmt.Errorf("%s has no active reply turn", id)
	}
	return err
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

func (s *Store) ClaimQueuedTurn(session storage.Session, prompt storage.QueuedMessage) error {
	queue, err := storage.MarshalQueuedMessages(session.QueuedMessages)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return threaddb.New(s.db).ClaimQueuedTurn(context.Background(), threaddb.ClaimQueuedTurnParams{
		ID: session.ID, QueuedMessages: queue, Title: nullDBString(session.Title),
		Turn: sessionTurnJSON(&storage.Turn{StartedAt: now, PendingMessage: &prompt}), StartedAtMs: timeToMs(now),
	})
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
		queued, err := s.queueDelivery(ctx, q, delivery)
		if err != nil {
			return err
		}
		if !queued && delivery.Otherwise != nil {
			queued, err = s.queueDelivery(ctx, q, *delivery.Otherwise)
			if err != nil {
				return err
			}
			delivery.Closes = nil
		}
		if !queued {
			return fmt.Errorf("delivery target %s is unavailable", delivery.SessionID)
		}
		for _, id := range delivery.Closes {
			if err := closeCommitment(ctx, botdb.New(tx), id, storage.CommitmentDone, "", at); err != nil && !errors.Is(err, storage.ErrCommitmentNotOpen) {
				return err
			}
		}
	}
	return tx.Commit()
}

// queueDelivery queues a delivery's message unless its thread is archived or
// gone.
func (s *Store) queueDelivery(ctx context.Context, q *threaddb.Queries, delivery storage.TurnDelivery) (bool, error) {
	message := delivery.Message
	message.ID = s.NewSessionID()
	raw, err := storage.MarshalQueuedMessage(&message)
	if err != nil {
		return false, err
	}
	updated, err := q.AppendQueuedTurn(ctx, threaddb.AppendQueuedTurnParams{ID: delivery.SessionID, Message: raw})
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
