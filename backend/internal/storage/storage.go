package storage

import (
	"time"

	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	RuntimeACP = "acp"
)

const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusError   = "error"
)

const (
	SourceLoopRun      = "loop_run"
	SourceMemoryDream  = "memory_dream"
	SourceMemorySearch = "memory_search"
	SourceBrowserTask  = "browser_task"
)

const (
	BlockTypeText              = "text"
	BlockTypeReasoning         = "reasoning"
	BlockTypeTool              = "tool"
	BlockTypeAttachment        = "attachment"
	BlockTypeQuote             = "quote"
	BlockTypeBrowserAnnotation = "browser_annotation"
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
	Type            string               `json:"type"`
	Agent           string               `json:"agent,omitempty"`
	SessionID       string               `json:"session_id,omitempty"`
	Cwd             string               `json:"cwd,omitempty"`
	ProjectPath     string               `json:"project_path,omitempty"`
	ArtifactSurface string               `json:"artifact_surface,omitempty"`
	MCPServerPolicy string               `json:"mcp_server_policy,omitempty"`
	Capabilities    *RuntimeCapabilities `json:"capabilities,omitempty"`
}

type RuntimeCapabilities struct {
	NativeGoal           bool `json:"native_goal,omitempty"`
	NativeGoalNegotiable bool `json:"native_goal_negotiable,omitempty"`
}

// Usage follows provider-facing token vocabulary: InputTokens includes cache
// reads/writes when the runtime reports them that way. Cache fields remain as
// details, not extra components to add on top of input.
type Usage struct {
	InputTokens           int64 `json:"input_tokens,omitempty"`
	CachedInputTokens     int64 `json:"cached_input_tokens,omitempty"` // cache reads
	CachedWriteTokens     int64 `json:"cached_write_tokens,omitempty"`
	OutputTokens          int64 `json:"output_tokens,omitempty"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens,omitempty"`
	TotalTokens           int64 `json:"total_tokens,omitempty"`
	// ContextTokens is the live context size after the most recent turn;
	// stores replace it on each usage write instead of accumulating.
	ContextTokens int64 `json:"context_tokens,omitempty"`
	// ContextWindowTokens is the model's context window as reported by the
	// runtime (e.g. usage_update "size"); replaced like ContextTokens, zero
	// when the runtime doesn't report it.
	ContextWindowTokens int64 `json:"context_window_tokens,omitempty"`
}

type UsageEvent struct {
	SessionID     string    `json:"session_id"`
	Runtime       string    `json:"runtime"`
	Agent         string    `json:"agent,omitempty"`
	ModelProvider string    `json:"model_provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	Usage         Usage     `json:"usage"`
	Source        string    `json:"source,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

const (
	UsageEventSourceTurn          = "turn"
	UsageEventSourceSessionImport = "session_import"
)

// ComponentTotal is the full processed-token count for a turn. Cache detail
// fields are already included in InputTokens.
func (u Usage) ComponentTotal() int64 {
	return u.InputTokens + u.OutputTokens
}

func (u Usage) IsZero() bool {
	return u == Usage{}
}

func (u Usage) Countable() bool {
	return u.InputTokens+u.CachedInputTokens+u.CachedWriteTokens+
		u.OutputTokens+u.ReasoningOutputTokens+u.TotalTokens > 0
}

// LiveContextTokens estimates the context occupied after a turn when the
// runtime didn't report it: everything sent plus what the model produced.
func (u Usage) LiveContextTokens() int64 {
	if u.ContextTokens > 0 {
		return u.ContextTokens
	}
	return u.ComponentTotal()
}

type Session struct {
	ID              string          `json:"id"`
	Slug            string          `json:"slug"`
	Title           string          `json:"title,omitempty"`
	ParentID        string          `json:"parent_id,omitempty"`
	Status          string          `json:"status"`
	Error           string          `json:"error,omitempty"`
	Runtime         string          `json:"runtime"`
	RuntimeRef      *RuntimeRef     `json:"runtime_ref,omitempty"`
	ModelProvider   string          `json:"model_provider,omitempty"`
	Model           string          `json:"model,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Usage           Usage           `json:"usage,omitempty"`
	QueuedMessages  []QueuedMessage `json:"queued_messages,omitempty"`
	PendingSteer    *QueuedMessage  `json:"pending_steer_message,omitempty"`
	SourceType      string          `json:"source_type,omitempty"`
	SourceID        string          `json:"source_id,omitempty"`
	Archived        bool            `json:"archived,omitempty"`
	Pinned          bool            `json:"pinned,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	LastAttentionAt time.Time       `json:"last_attention_at"`
}

func MarkSessionAttention(session *Session, at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	session.LastAttentionAt = at
}

func SessionAttentionAt(session Session) time.Time {
	if !session.LastAttentionAt.IsZero() {
		return session.LastAttentionAt
	}
	if !session.UpdatedAt.IsZero() {
		return session.UpdatedAt
	}
	return session.CreatedAt
}

type Block struct {
	Type       string      `json:"type"`
	Text       string      `json:"text,omitempty"`
	ID         string      `json:"id,omitempty"`
	Name       string      `json:"name,omitempty"`
	URI        string      `json:"uri,omitempty"`
	MimeType   string      `json:"mime_type,omitempty"`
	Size       int64       `json:"size,omitempty"`
	ServerPath string      `json:"server_path,omitempty"`
	InputJSON  string      `json:"input_json,omitempty"`
	Result     string      `json:"result,omitempty"`
	MediaRefs  []media.Ref `json:"media_refs,omitempty"`
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
	ID              string                        `json:"id"`
	Slug            string                        `json:"slug"`
	Title           string                        `json:"title,omitempty"`
	ParentID        string                        `json:"parent_id,omitempty"`
	ACPAgent        string                        `json:"acp_agent"`
	ACPSession      string                        `json:"acp_session"`
	Cwd             string                        `json:"cwd,omitempty"`
	ModelProvider   string                        `json:"model_provider,omitempty"`
	Model           string                        `json:"model,omitempty"`
	ReasoningEffort string                        `json:"reasoning_effort,omitempty"`
	State           string                        `json:"state"`
	StopReason      string                        `json:"stop_reason,omitempty"`
	Assistant       string                        `json:"assistant,omitempty"`
	Thought         string                        `json:"thought,omitempty"`
	Plan            []sessionevents.ACPPlanEntry  `json:"plan,omitempty"`
	ToolCalls       []sessionevents.ACPToolCall   `json:"tool_calls,omitempty"`
	Permissions     []sessionevents.ACPPermission `json:"permissions,omitempty"`
	Modes           sessionevents.ACPModeState    `json:"modes,omitempty"`
	Error           string                        `json:"error,omitempty"`
	ParentVisible   bool                          `json:"parent_visible,omitempty"`
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`
	LastEventAt     time.Time                     `json:"last_event_at,omitzero"`
	LastToolAt      time.Time                     `json:"last_tool_at,omitzero"`
}

type CreateSession struct {
	Slug            string
	Title           string
	ParentID        string
	Runtime         string
	RuntimeRef      *RuntimeRef
	ModelProvider   string
	Model           string
	ReasoningEffort string
	SourceType      string
	SourceID        string
}

type SessionFilter struct {
	ParentID        string
	ParentOnly      bool
	RootOnly        bool
	Runtime         string
	IncludeChildren bool
	SourceType      string
	SourceID        string
	IncludeSourced  bool
	// UpdatedSince selects only sessions updated strictly after this time.
	UpdatedSince time.Time
	// Archived selects only archived sessions; by default they are excluded.
	Archived bool
	Limit    int
}

func SessionMatchesFilter(session Session, filter SessionFilter) bool {
	if filter.RootOnly && session.ParentID != "" {
		return false
	}
	if filter.ParentOnly && session.ParentID != filter.ParentID {
		return false
	}
	if !filter.IncludeChildren && !filter.ParentOnly && !filter.RootOnly && filter.ParentID == "" && session.ParentID != "" {
		return false
	}
	if filter.ParentID != "" && session.ParentID != filter.ParentID {
		return false
	}
	if filter.Runtime != "" && session.Runtime != filter.Runtime {
		return false
	}
	if filter.SourceType != "" && session.SourceType != filter.SourceType {
		return false
	}
	if filter.SourceID != "" && session.SourceID != filter.SourceID {
		return false
	}
	if filter.SourceType == "" && filter.SourceID == "" && !filter.IncludeSourced && session.SourceType != "" {
		return false
	}
	if !filter.UpdatedSince.IsZero() && !session.UpdatedAt.After(filter.UpdatedSince) {
		return false
	}
	return session.Archived == filter.Archived
}
