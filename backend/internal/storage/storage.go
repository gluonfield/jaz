package storage

import (
	"time"

	"github.com/wins/jaz/backend/internal/provider"
)

const (
	RuntimeNative = "native"
	RuntimeACP    = "acp"
)

type RuntimeRef struct {
	Type      string `json:"type"`
	Agent     string `json:"agent,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type Session struct {
	ID         string      `json:"id"`
	Slug       string      `json:"slug"`
	Title      string      `json:"title,omitempty"`
	ParentID   string      `json:"parent_id,omitempty"`
	Runtime    string      `json:"runtime"`
	RuntimeRef *RuntimeRef `json:"runtime_ref,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

type CreateSession struct {
	Slug       string
	Title      string
	ParentID   string
	Runtime    string
	RuntimeRef *RuntimeRef
}

type SessionFilter struct {
	ParentID        string
	ParentOnly      bool
	RootOnly        bool
	Runtime         string
	IncludeChildren bool
	Limit           int
}

type Store interface {
	NewSessionID() string
	CreateSession(input CreateSession) (Session, error)
	EnsureSession(id string) error
	LoadSession(ref string) (Session, error)
	SaveSession(session Session) error
	ListSessions(filter SessionFilter) ([]Session, error)
	LastRootSession() (Session, error)
	LoadMessages(id string) ([]provider.Message, error)
	SaveMessages(id string, messages []provider.Message) error
}
