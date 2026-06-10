package sqlite

import (
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Store) mirrorSession(session storage.Session) {
	if s.mirror != nil {
		_ = s.mirror.SaveSession(session)
	}
}

func (s *Store) mirrorMessages(id string, messages []provider.Message) {
	if s.mirror != nil {
		_ = s.mirror.SaveMessages(id, messages)
	}
}
