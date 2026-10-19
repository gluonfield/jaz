package storage

import (
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type SessionStore interface {
	SessionGoalStore
	NewSessionID() string
	CreateSession(input CreateSession) (Session, error)
	EnsureSession(id string) error
	LoadSession(ref string) (Session, error)
	SaveSession(session Session) error
	UpdateSessionModel(id, model, effort string) error
	ReplaceRuntimeSessionID(id, oldID, newID string) (bool, error)
	CompleteSession(id string, completedAt time.Time) error
	FinishSessionTurn(id, status, errorMessage string, at time.Time, deliveries []TurnDelivery) error
	// QueueTurn queues a delivery and closes the commitments it delivers.
	QueueTurn(TurnDelivery) error
	// ClearBackgroundWork forgets which threads' agents held live background
	// tasks.
	ClearBackgroundWork() error
	ClaimQueuedTurn(Session, QueuedMessage) error
	SetThreadUnread(id string, unread bool) error
	TouchSessionAttention(id string) error
	SetArchived(id string, archived bool) error
	SetPinned(id string, pinned bool) error
	UpdateSessionTitle(id, title string) error
	UpdateSessionStatus(id, status, errorMessage string, attentionAt time.Time) error
	ListSessions(filter SessionFilter) ([]Session, error)
	LastRootSession() (Session, error)
}

type SessionTranscriptReader interface {
	HasAgentTranscript(id string) (bool, error)
}

type MessageStore interface {
	LoadMessages(id string) ([]provider.Message, error)
	SaveMessages(id string, messages []provider.Message) error
	MessageAppender
}

type MessageAppender interface {
	AppendMessages(id string, messages ...provider.Message) error
}

type MessageRecordAppender interface {
	AppendMessageRecords(id string, messages ...Message) error
}

type SessionEventReader interface {
	LoadSessionEvents(id string) ([]sessionevents.Event, error)
	LoadSessionEventsAfter(id string, afterSeq int64) ([]sessionevents.Event, error)
}

type SessionEventAppender interface {
	AppendSessionEvents(id string, events ...sessionevents.Event) error
}

type SessionEventStore interface {
	SessionEventReader
	SessionEventAppender
	LoadSessionOverviewEvents(id string) ([]sessionevents.Event, error)
}

// UsageEvents returns [since, until); a zero until leaves the upper bound open.
type UsageEventStore interface {
	UsageEvents(since, until time.Time) ([]UsageEvent, error)
}

type FeedStore interface {
	LoadFeed() ([]FeedItem, error)
	LoadFeedCompletions() ([]FeedCompletion, error)
}

type Store interface {
	SessionStore
	SessionTranscriptReader
	MessageStore
	SessionEventStore
	SettingsStorage
	UsageEventStore
}
