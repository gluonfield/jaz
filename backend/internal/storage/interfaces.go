package storage

import (
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type SessionStore interface {
	NewSessionID() string
	CreateSession(input CreateSession) (Session, error)
	EnsureSession(id string) error
	LoadSession(ref string) (Session, error)
	SaveSession(session Session) error
	SetArchived(id string, archived bool) error
	ListSessions(filter SessionFilter) ([]Session, error)
	LastRootSession() (Session, error)
}

type MessageStore interface {
	LoadMessages(id string) ([]provider.Message, error)
	SaveMessages(id string, messages []provider.Message) error
	MessageAppender
}

type MessageAppender interface {
	AppendMessages(id string, messages ...provider.Message) error
}

type ActivityStore interface {
	LoadActivity(id string) ([]ActivityEntry, error)
	SaveActivity(id string, activity []ActivityEntry) error
	ActivityUpserter
}

type ActivityUpserter interface {
	UpsertActivity(id string, entry ActivityEntry) error
}

type SessionEventReader interface {
	LoadSessionEvents(id string) ([]sessionevents.Event, error)
}

type SessionEventAppender interface {
	AppendSessionEvents(id string, events ...sessionevents.Event) error
}

type SessionEventStore interface {
	SessionEventReader
	SessionEventAppender
}

type ACPStateStore interface {
	LoadACPState(id string) (ACPState, error)
	SaveACPState(id string, state ACPState) error
}

type Store interface {
	SessionStore
	MessageStore
	ActivityStore
	SessionEventStore
	ACPStateStore
}
