package sqlite

import (
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

type sessionExportMirror interface {
	SaveSessionSnapshot(storage.Session) error
	SaveMessages(string, []provider.Message) error
	AppendMessages(string, ...provider.Message) error
}

func (s *Store) mirrorSession(session storage.Session) {
	if s.exportMirror != nil {
		_ = s.exportMirror.SaveSessionSnapshot(session)
	}
}

func (s *Store) mirrorMessages(id string, messages []provider.Message) {
	if s.exportMirror != nil {
		_ = s.exportMirror.SaveMessages(id, messages)
	}
}

func (s *Store) mirrorMessageRecords(id string, records []storage.Message) {
	if s.exportMirror == nil {
		return
	}
	if messages, err := providerMessagesFromRecords(records); err == nil {
		_ = s.exportMirror.AppendMessages(id, messages...)
	}
}
