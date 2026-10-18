package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) StartSessionTurn(id string, turn storage.Turn) error {
	s.writeMu.Lock()
	err := threaddb.New(s.db).StartSessionTurn(context.Background(), threaddb.StartSessionTurnParams{
		ID: id, Turn: sessionTurnJSON(&turn), StartedAtMs: timeToMs(time.Now().UTC()),
	})
	s.writeMu.Unlock()
	if err == nil {
		if current, loadErr := s.LoadSession(id); loadErr == nil {
			s.mirrorSession(current)
		}
	}
	return err
}

func (s *Store) AppendTurnReply(id, message string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	updated, err := threaddb.New(s.db).AppendTurnReply(context.Background(), threaddb.AppendTurnReplyParams{ID: id, Message: message})
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return threaddb.New(s.db).ClaimQueuedTurn(context.Background(), threaddb.ClaimQueuedTurnParams{
		ID: session.ID, QueuedMessages: queue, Title: nullDBString(session.Title),
		Turn: sessionTurnJSON(&storage.Turn{PendingMessage: &prompt}), StartedAtMs: timeToMs(time.Now().UTC()),
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
		message := delivery.Message
		message.ID = s.NewSessionID()
		raw, err := storage.MarshalQueuedMessage(&message)
		if err != nil {
			return err
		}
		updated, err := q.AppendQueuedTurn(ctx, threaddb.AppendQueuedTurnParams{ID: delivery.SessionID, Message: raw})
		if err != nil {
			return err
		}
		if updated == 0 {
			return fmt.Errorf("delivery target %s is unavailable", delivery.SessionID)
		}
	}
	return tx.Commit()
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
