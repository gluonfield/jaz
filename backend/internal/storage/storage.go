package storage

import (
	"time"

	"github.com/wins/jaz/backend/internal/provider"
)

const (
	RuntimeNative = "native"
	RuntimeACP    = "acp"
)

const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusError   = "error"
)

type RuntimeRef struct {
	Type      string `json:"type"`
	Agent     string `json:"agent,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type Usage struct {
	InputTokens           int64 `json:"input_tokens,omitempty"`
	CachedInputTokens     int64 `json:"cached_input_tokens,omitempty"`
	OutputTokens          int64 `json:"output_tokens,omitempty"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens,omitempty"`
	TotalTokens           int64 `json:"total_tokens,omitempty"`
}

type Session struct {
	ID              string      `json:"id"`
	Slug            string      `json:"slug"`
	Title           string      `json:"title,omitempty"`
	ParentID        string      `json:"parent_id,omitempty"`
	Status          string      `json:"status"`
	Runtime         string      `json:"runtime"`
	RuntimeRef      *RuntimeRef `json:"runtime_ref,omitempty"`
	ModelProvider   string      `json:"model_provider,omitempty"`
	Model           string      `json:"model,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
	Usage           Usage       `json:"usage,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type Block struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	InputJSON string `json:"input_json,omitempty"`
	Result    string `json:"result,omitempty"`
}

type Message struct {
	ThreadID  string    `json:"thread_id,omitempty"`
	Seq       int64     `json:"seq"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Reasoning string    `json:"reasoning,omitempty"`
	Blocks    []Block   `json:"blocks"`
	CreatedAt time.Time `json:"created_at"`
}

type ActivityEntry struct {
	ID     string    `json:"id,omitempty"`
	Kind   string    `json:"kind"`
	Text   string    `json:"text,omitempty"`
	Status string    `json:"status,omitempty"`
	At     time.Time `json:"at"`
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
	AppendMessages(id string, messages ...provider.Message) error
	LoadActivity(id string) ([]ActivityEntry, error)
	SaveActivity(id string, activity []ActivityEntry) error
	UpsertActivity(id string, entry ActivityEntry) error
}
