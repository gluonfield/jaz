package jsonstore

import (
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
	session.Status = storage.StatusRunning
	session.Error = ""
	session.Turn = &turn
	storage.MarkSessionAttention(&session, time.Now().UTC())
	return s.saveSession(session)
}
