package jsonstore

import (
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Store) StartSessionTurn(id string, turn storage.Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	turn.StartedAt = time.Now().UTC()
	session.Status = storage.StatusRunning
	session.Error = ""
	session.Turn = &turn
	storage.MarkSessionAttention(&session, turn.StartedAt)
	return s.saveSession(session)
}

func (s *Store) ClaimQueuedTurn(session storage.Session, prompt storage.QueuedMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.Status = storage.StatusRunning
	session.Error = ""
	session.Turn = &storage.Turn{StartedAt: time.Now().UTC(), PendingMessage: &prompt}
	storage.MarkSessionAttention(&session, session.Turn.StartedAt)
	return s.saveSession(session)
}

func (s *Store) SetTurnIntent(id string, goalRequested, parentVisible, notifyParent bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	if session.Turn == nil || session.Status != storage.StatusRunning {
		return fmt.Errorf("%s has no active turn", id)
	}
	session.Turn.GoalRequested = goalRequested
	session.Turn.ParentVisible = parentVisible
	session.Turn.NotifyParent = notifyParent
	return s.saveSession(session)
}

func (s *Store) ResetTurnHops(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil || session.Status != storage.StatusRunning || session.Turn == nil || session.Turn.Output == nil {
		return err
	}
	session.Turn.Output.Hops = 0
	return s.saveSession(session)
}

func (s *Store) FinishSessionTurn(id, status, errorMessage string, at time.Time, deliveries []storage.TurnDelivery) error {
	if len(deliveries) > 0 {
		return fmt.Errorf("JSON storage does not support atomic turn delivery")
	}
	if status == storage.StatusIdle {
		return s.CompleteSession(id, at)
	}
	return s.UpdateSessionStatus(id, status, errorMessage, at)
}
