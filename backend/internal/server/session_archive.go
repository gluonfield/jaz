package server

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Server) setSessionArchivedState(sessionID string, archived bool) (storage.Session, error) {
	if err := s.Store.SetArchived(sessionID, archived); err != nil {
		return storage.Session{}, err
	}
	return s.Store.LoadSession(sessionID)
}

func (s *Server) pruneManagedWorktreesSoon() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		s.PruneManagedWorktrees(ctx)
	}()
}
