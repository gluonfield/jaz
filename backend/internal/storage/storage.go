package storage

import (
	"errors"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/media"
)

var ErrSessionNotFound = errors.New("session not found")

const (
	RuntimeACP = "acp"
)

const (
	StatusIdle        = "idle"
	StatusRunning     = "running"
	StatusError       = "error"
	StatusInterrupted = "interrupted"
)

const (
	SourceLoopRun = "loop_run"
	SourceBot     = "bot"
	// SourceBotWorker tags a thread a bot started for background work; its
	// source id is the bot.
	SourceBotWorker = "bot_worker"
	// SourceBotMember tags the thread a bot takes its turns in for one group;
	// its source id is the bot.
	SourceBotMember    = "bot_member"
	SourceMemoryDream  = "memory_dream"
	SourceMemorySearch = "memory_search"
	SourceMemorySource = "memory_source"

	// LegacySourceBrowserTask is retained only for persisted usage history.
	LegacySourceBrowserTask = "browser_task"
)

const (
	BlockTypeText              = "text"
	BlockTypeVoiceContext      = "voice_context"
	BlockTypeReasoning         = "reasoning"
	BlockTypeTool              = "tool"
	BlockTypeAttachment        = "attachment"
	BlockTypeQuote             = "quote"
	BlockTypeBrowserAnnotation = "browser_annotation"
)

func SessionStatusForACPState(state, stopReason string) string {
	if state == "cancelled" && stopReason == "server_shutdown" {
		return StatusInterrupted
	}
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
	Type            string `json:"type"`
	Agent           string `json:"agent,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	Cwd             string `json:"cwd,omitempty"`
	ProjectPath     string `json:"project_path,omitempty"`
	ArtifactSurface string `json:"artifact_surface,omitempty"`
	MCPServerPolicy string `json:"mcp_server_policy,omitempty"`
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
	SessionID     string `json:"session_id"`
	Runtime       string `json:"runtime"`
	Agent         string `json:"agent,omitempty"`
	ModelProvider string `json:"model_provider,omitempty"`
	Model         string `json:"model,omitempty"`
	Usage         Usage  `json:"usage"`
	Source        string `json:"source,omitempty"`
	// SourceType is the originating session's source type, empty for interactive chat.
	SourceType string    `json:"source_type,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
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

type Session struct {
	ID              string          `json:"id"`
	Slug            string          `json:"slug"`
	Title           string          `json:"title,omitempty"`
	ManualTitle     bool            `json:"manual_title,omitempty"`
	TitleLocked     bool            `json:"title_locked,omitempty"`
	ParentID        string          `json:"parent_id,omitempty"`
	Status          string          `json:"status"`
	Turn            *Turn           `json:"turn,omitempty"`
	Error           string          `json:"error,omitempty"`
	Runtime         string          `json:"runtime"`
	RuntimeRef      *RuntimeRef     `json:"runtime_ref,omitempty"`
	ModelProvider   string          `json:"model_provider,omitempty"`
	Model           string          `json:"model,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Goal            *goal.State     `json:"goal,omitempty"`
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
	// CompletedAt is when the thread's last successful turn ended, and
	// CompletedTurnStartedAt when it started.
	CompletedAt            time.Time `json:"completed_at,omitzero"`
	CompletedTurnStartedAt time.Time `json:"completed_turn_started_at,omitzero"`
	// PromptDigest identifies the version of the thread's own prompt its
	// agent was shown, as of its last successful turn; empty when unknown.
	PromptDigest string `json:"prompt_digest,omitempty"`
	Unread       bool   `json:"unread,omitempty"`
}

// Owner is the thread a thread works for: the bot whose subtask, group seat
// or routine run it is, or else itself.
func (s Session) Owner() string {
	switch s.SourceType {
	case SourceBotWorker, SourceBotMember:
		return s.SourceID
	case SourceLoopRun:
		return s.ParentID
	}
	return s.ID
}

type Turn struct {
	StartedAt       time.Time      `json:"started_at,omitzero"`
	PlanRequested   bool           `json:"plan_requested,omitempty"`
	GoalRequested   bool           `json:"goal_requested,omitempty"`
	ActiveOperation string         `json:"active_operation,omitempty"`
	ParentVisible   bool           `json:"parent_visible,omitempty"`
	NotifyParent    bool           `json:"notify_parent,omitempty"`
	AllowSilence    bool           `json:"allow_silence,omitempty"`
	Output          *TurnOutput    `json:"output,omitempty"`
	PendingMessage  *QueuedMessage `json:"pending_message,omitempty"`
	// ParentOutput is the output of the parent turn this one's result opens:
	// the exchange with another bot the parent was answering when it started
	// this turn.
	ParentOutput *TurnOutput `json:"parent_output,omitempty"`
	// PromptDigest identifies the version of the thread's own prompt its
	// agent holds once this turn has started.
	PromptDigest string `json:"prompt_digest,omitempty"`
	// Retries counts the continuations Jaz has sent after provider failures
	// ended this turn's earlier prompts.
	Retries int `json:"retries,omitempty"`
}

type TurnOutput struct {
	ReplyTo string   `json:"reply_to,omitempty"`
	Replies []string `json:"replies,omitempty"`
	// Run is the routine run the turn does; the turn's end finishes it.
	Run string `json:"run,omitempty"`
	// Hops counts the messages bots sent each other, without the user
	// writing, that led to this turn.
	Hops int `json:"hops,omitempty"`
	// Shown records that the turn sent a message or presented an app in its
	// own chat.
	Shown bool `json:"shown,omitempty"`
	// Done are the commitments the replies deliver; they close as the replies
	// reach ReplyTo.
	Done []int64 `json:"done,omitempty"`
}

// Continuation is the output of a later turn that goes on answering the bot
// this turn answers; nil when it answers none. A routine run closes with its
// turn, so a later turn answers its bot without it.
func (o *TurnOutput) Continuation() *TurnOutput {
	if o == nil || o.ReplyTo == "" {
		return nil
	}
	return &TurnOutput{ReplyTo: o.ReplyTo, Hops: o.Hops}
}

// LoopRun is the routine run a turn with this output does, if any.
func (o *TurnOutput) LoopRun() string {
	if o == nil {
		return ""
	}
	return o.Run
}

// TurnDelivery is a message queued for a thread's next turn. A finished
// turn's delivery to a thread that is archived or gone queues its Fallback in
// the finished thread, or else is dropped: nobody is left to tell.
type TurnDelivery struct {
	SessionID string
	Message   QueuedMessage
	// Closes are the commitments that close as done once the message is
	// queued.
	Closes []int64
	// Wake queues the message to the bot SessionID only while some of its
	// commitments stall without it having been woken about them, and records
	// on those that it was.
	Wake     bool
	Fallback *QueuedMessage
}

type FeedItem struct {
	ID        string
	Slug      string
	Title     string
	ParentID  string
	ReplyText string
	ReplyAt   time.Time
}

type FeedCompletion struct {
	ID          string
	Slug        string
	Title       string
	CompletedAt time.Time
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
	Comment    string      `json:"comment,omitempty"`
	ID         string      `json:"id,omitempty"`
	RequestID  string      `json:"request_id,omitempty"`
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
	// FromAgent marks a user-role message an agent wrote, as when it sends
	// another thread a message, rather than the user.
	FromAgent bool `json:"from_agent,omitempty"`
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
	// Commitment is the open commitment of the bot SourceID that the thread
	// serves.
	Commitment int64
}

type SessionFilter struct {
	Query           string
	After           *SessionPosition
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

type SessionPosition struct {
	AttentionAt time.Time
	ID          string
}

func SessionMatchesFilter(session Session, filter SessionFilter) bool {
	if filter.Query != "" && !strings.Contains(strings.ToLower(session.Title+"\n"+session.Slug), strings.ToLower(filter.Query)) {
		return false
	}
	if filter.After != nil {
		at := SessionAttentionAt(session).UnixMilli()
		after := filter.After.AttentionAt.UnixMilli()
		if at > after || (at == after && session.ID <= filter.After.ID) {
			return false
		}
	}
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
