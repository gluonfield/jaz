package storage

import (
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
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

func SessionStatusForACPState(state string) string {
	switch state {
	case "starting", "running":
		return StatusRunning
	case "idle", "cancelled":
		return StatusIdle
	case "failed":
		return StatusError
	default:
		return ""
	}
}

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
	Error           string      `json:"error,omitempty"`
	Runtime         string      `json:"runtime"`
	RuntimeRef      *RuntimeRef `json:"runtime_ref,omitempty"`
	ModelProvider   string      `json:"model_provider,omitempty"`
	Model           string      `json:"model,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
	Usage           Usage       `json:"usage,omitempty"`
	Archived        bool        `json:"archived,omitempty"`
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

type ACPState struct {
	ID            string                        `json:"id"`
	Slug          string                        `json:"slug"`
	Title         string                        `json:"title,omitempty"`
	ParentID      string                        `json:"parent_id,omitempty"`
	ACPAgent      string                        `json:"acp_agent"`
	ACPSession    string                        `json:"acp_session"`
	Cwd           string                        `json:"cwd,omitempty"`
	State         string                        `json:"state"`
	StopReason    string                        `json:"stop_reason,omitempty"`
	Assistant     string                        `json:"assistant,omitempty"`
	Thought       string                        `json:"thought,omitempty"`
	Plan          []sessionevents.ACPPlanEntry  `json:"plan,omitempty"`
	ToolCalls     []sessionevents.ACPToolCall   `json:"tool_calls,omitempty"`
	Permissions   []sessionevents.ACPPermission `json:"permissions,omitempty"`
	Modes         sessionevents.ACPModeState    `json:"modes,omitempty"`
	Error         string                        `json:"error,omitempty"`
	ParentVisible bool                          `json:"parent_visible,omitempty"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
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
	// Archived selects only archived sessions; by default they are excluded.
	Archived bool
	Limit    int
}
