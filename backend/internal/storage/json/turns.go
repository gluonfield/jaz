package jsonstore

import (
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Store) StartSessionTurn(id string, turn storage.Turn, messages ...storage.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	if len(messages) > 0 {
		existing, err := s.loadMessages(id)
		if err != nil {
			return err
		}
		for _, message := range messages {
			existing = append(existing, provider.UserMessage(message.Content))
		}
		if err := s.saveMessages(id, existing); err != nil {
			return err
		}
	}
	if turn.StartedAt.IsZero() {
		turn.StartedAt = time.Now().UTC()
	}
	session.Status = storage.StatusRunning
	session.Error = ""
	session.Turn = &turn
	storage.MarkSessionAttention(&session, turn.StartedAt)
	return s.saveSession(session)
}

func (s *Store) ClaimTurn(id string, turn storage.Turn) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil || session.Status == storage.StatusRunning {
		return false, err
	}
	turn.StartedAt = time.Now().UTC()
	session.Status = storage.StatusRunning
	session.Error = ""
	session.Turn = &turn
	storage.MarkSessionAttention(&session, turn.StartedAt)
	return true, s.saveSession(session)
}

func (s *Store) OweTurn(id string, owed storage.Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil || session.Turn == nil || session.Status != storage.StatusRunning {
		return err
	}
	turn := session.Turn.Owing(owed)
	session.Turn = &turn
	return s.saveSession(session)
}

func (s *Store) SetTurnPromptState(id, agentSessionID string, startedAt time.Time, state storage.PromptState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	if session.Status != storage.StatusRunning || session.Turn == nil || session.Turn.Prompt == nil ||
		session.RuntimeRef == nil || session.RuntimeRef.SessionID != "" && session.RuntimeRef.SessionID != agentSessionID || !session.Turn.StartedAt.Equal(startedAt) {
		return fmt.Errorf("%s: turn or agent session changed before prompt state was recorded", id)
	}
	session.RuntimeRef.SessionID = agentSessionID
	if session.Turn.Prompt.State != storage.PromptConfirmed {
		session.Turn.Prompt.State = state
	}
	return s.saveSession(session)
}

func (s *Store) ClaimQueuedTurn(session storage.Session, prompt storage.QueuedMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, err := s.loadSessionByID(session.ID); err == nil && current.Status == storage.StatusRunning {
		return storage.ErrThreadRunning
	}
	session.Status = storage.StatusRunning
	session.Error = ""
	session.Turn = prompt.Turn(time.Now().UTC())
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

func (s *Store) SetTurnRetries(id string, retries int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	if session.Turn == nil || session.Status != storage.StatusRunning {
		return fmt.Errorf("%s has no active turn", id)
	}
	session.Turn.Retries = retries
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

// QueueTurn queues a delivery for its thread's next turn; JSON storage keeps
// no commitments for it to close.
func (s *Store) QueueTurn(delivery storage.TurnDelivery) error {
	if len(delivery.Closes) > 0 || delivery.Wake {
		return fmt.Errorf("JSON storage does not keep commitments")
	}
	if delivery.Fallback != nil {
		return fmt.Errorf("a queued turn has no fallback")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(delivery.SessionID)
	if err != nil {
		return err
	}
	if session.Archived {
		return fmt.Errorf("%s: %w", delivery.SessionID, storage.ErrSessionNotFound)
	}
	message := delivery.Message
	message.ID = s.NewSessionID()
	session.QueuedMessages = append(session.QueuedMessages, message)
	return s.saveSession(session)
}
