package sessionevents

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/messagepayload"
)

const (
	TypeArtifact         = "artifact"
	TypeSession          = "session"
	TypeLoopCreated      = "loop_created"
	TypeSideChatMessage  = "side_chat_message"
	TypeProviderSubagent = "provider_subagent"
	TypeGoalUpdate       = "goal_update"
)

type Event struct {
	Seq              int64                  `json:"seq,omitempty"`
	SessionID        string                 `json:"session_id"`
	Type             string                 `json:"type"`
	Content          string                 `json:"content,omitempty"`
	ACP              *ACPEvent              `json:"acp,omitempty"`
	Plan             *PlanEvent             `json:"plan,omitempty"`
	Goal             *GoalEvent             `json:"goal,omitempty"`
	Permission       *ACPPermission         `json:"permission,omitempty"`
	Artifact         *ArtifactEvent         `json:"artifact,omitempty"`
	LoopCreated      *LoopCreatedEvent      `json:"loop_created,omitempty"`
	SideChat         *SideChatEvent         `json:"side_chat,omitempty"`
	ProviderSubagent *ProviderSubagentEvent `json:"provider_subagent,omitempty"`
	At               time.Time              `json:"at"`
}

type SideChatEvent struct {
	ID              string                          `json:"id"`
	Command         string                          `json:"command,omitempty"`
	ParentSessionID string                          `json:"parent_session_id,omitempty"`
	ThreadID        string                          `json:"thread_id,omitempty"`
	Role            string                          `json:"role"`
	Content         string                          `json:"content"`
	Status          string                          `json:"status,omitempty"`
	Contexts        []messagepayload.MessageContext `json:"contexts,omitempty"`
	Attachments     []messagepayload.Attachment     `json:"attachments,omitempty"`
}

type ProviderSubagentEvent struct {
	Provider        string `json:"provider,omitempty"`
	ID              string `json:"id"`
	ThreadID        string `json:"thread_id,omitempty"`
	ParentID        string `json:"parent_id,omitempty"`
	Name            string `json:"name,omitempty"`
	Role            string `json:"role,omitempty"`
	Status          string `json:"status,omitempty"`
	Summary         string `json:"summary,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	StartedAtMs     int64  `json:"started_at_ms,omitempty"`
	CompletedAtMs   int64  `json:"completed_at_ms,omitempty"`
}

type ArtifactEvent struct {
	Title           string   `json:"title"`
	WidgetCode      string   `json:"widget_code"`
	LoadingMessages []string `json:"loading_messages,omitempty"`
	ArtifactType    string   `json:"artifact_type,omitempty"`
}

// LoopCreatedEvent renders a card in the thread when a loop is created, linking
// to the loop and to any boards its widget was pinned to. Like ArtifactEvent it
// has no dedicated storage column: it round-trips through the content column.
type LoopCreatedEvent struct {
	LoopID    string         `json:"loop_id"`
	LoopName  string         `json:"loop_name"`
	Schedule  string         `json:"schedule,omitempty"`
	Timezone  string         `json:"timezone,omitempty"`
	NextRunAt time.Time      `json:"next_run_at,omitzero"`
	Agent     string         `json:"agent,omitempty"`
	Status    string         `json:"status,omitempty"`
	Boards    []LoopBoardRef `json:"boards,omitempty"`
}

type LoopBoardRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (e *Event) NormalizePayload() {
	if e == nil {
		return
	}
	switch e.Type {
	case TypeArtifact:
		if e.Artifact != nil {
			e.Content = ""
			return
		}
		if e.Content == "" {
			return
		}
		var artifact ArtifactEvent
		if err := json.Unmarshal([]byte(e.Content), &artifact); err == nil && artifact.Title != "" && artifact.WidgetCode != "" {
			e.Artifact = &artifact
			e.Content = ""
		}
	case TypeLoopCreated:
		if e.LoopCreated != nil {
			e.Content = ""
			return
		}
		if e.Content == "" {
			return
		}
		var loop LoopCreatedEvent
		if err := json.Unmarshal([]byte(e.Content), &loop); err == nil && loop.LoopID != "" {
			e.LoopCreated = &loop
			e.Content = ""
		}
	case TypeProviderSubagent:
		if e.ProviderSubagent != nil {
			e.Content = ""
			return
		}
		if e.Content == "" {
			return
		}
		var subagent ProviderSubagentEvent
		if err := json.Unmarshal([]byte(e.Content), &subagent); err == nil && subagent.ID != "" {
			e.ProviderSubagent = &subagent
			e.Content = ""
		}
	case TypeSideChatMessage:
		if e.SideChat != nil {
			e.Content = ""
			return
		}
		if e.Content == "" {
			return
		}
		var side SideChatEvent
		if err := json.Unmarshal([]byte(e.Content), &side); err == nil && side.ID != "" {
			e.SideChat = &side
			e.Content = ""
		}
	case TypeGoalUpdate:
		if e.Goal != nil {
			e.Content = ""
			return
		}
		if e.Content == "" {
			return
		}
		var goal GoalEvent
		if err := json.Unmarshal([]byte(e.Content), &goal); err == nil && goal.Status != "" {
			e.Goal = &goal
			e.Content = ""
		}
	}
}

func (e Event) StorageContent() string {
	switch e.Type {
	case TypeArtifact:
		if e.Artifact == nil {
			return e.Content
		}
		if data, err := json.Marshal(e.Artifact); err == nil {
			return string(data)
		}
	case TypeLoopCreated:
		if e.LoopCreated == nil {
			return e.Content
		}
		if data, err := json.Marshal(e.LoopCreated); err == nil {
			return string(data)
		}
	case TypeProviderSubagent:
		if e.ProviderSubagent == nil {
			return e.Content
		}
		if data, err := json.Marshal(e.ProviderSubagent); err == nil {
			return string(data)
		}
	case TypeSideChatMessage:
		if e.SideChat == nil {
			return e.Content
		}
		if data, err := json.Marshal(e.SideChat); err == nil {
			return string(data)
		}
	case TypeGoalUpdate:
		if e.Goal == nil {
			return e.Content
		}
		if data, err := json.Marshal(e.Goal); err == nil {
			return string(data)
		}
	}
	return e.Content
}

type ACPEvent struct {
	ID              string          `json:"id"`
	Slug            string          `json:"slug"`
	Title           string          `json:"title,omitempty"`
	ParentID        string          `json:"parent_id,omitempty"`
	Agent           string          `json:"agent"`
	SessionID       string          `json:"session_id"`
	ModelProvider   string          `json:"model_provider,omitempty"`
	Model           string          `json:"model,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	State           string          `json:"state"`
	StopReason      string          `json:"stop_reason,omitempty"`
	Assistant       string          `json:"assistant,omitempty"`
	Thought         string          `json:"thought,omitempty"`
	Error           string          `json:"error,omitempty"`
	Modes           ACPModeState    `json:"modes,omitzero"`
	Plan            []PlanEntry     `json:"plan,omitempty"`
	ToolCalls       []ACPToolCall   `json:"tool_calls,omitempty"`
	Permissions     []ACPPermission `json:"permissions,omitempty"`
	LastEventAt     time.Time       `json:"last_event_at,omitzero"`
	LastToolAt      time.Time       `json:"last_tool_at,omitzero"`
}

func (e ACPEvent) MarshalJSON() ([]byte, error) {
	type acpEvent ACPEvent
	if e.Plan != nil {
		return json.Marshal(struct {
			acpEvent
			Plan []PlanEntry `json:"plan"`
		}{
			acpEvent: acpEvent(e),
			Plan:     e.Plan,
		})
	}
	return json.Marshal(struct {
		acpEvent
		Plan []PlanEntry `json:"plan,omitempty"`
	}{
		acpEvent: acpEvent(e),
	})
}

// SlimForStorage returns a copy without session-constant fields: repeating
// titles, model metadata, and the mode catalog on every stored row dominated
// transcript payloads (~70-90% of bytes on tool-heavy threads). The slug stays
// embedded as a durable label fallback, and task-bearing events keep the
// current/plan mode ids that approval state reads. Migration 0014 applies the
// same rule to historical rows.
func (e *ACPEvent) SlimForStorage() *ACPEvent {
	if e == nil {
		return nil
	}
	slim := *e
	slim.Title = ""
	slim.ModelProvider = ""
	slim.Model = ""
	slim.ReasoningEffort = ""
	slim.Modes.AvailableModes = nil
	if len(slim.Plan) == 0 {
		slim.Modes = ACPModeState{}
	}
	return &slim
}

type PlanEvent struct {
	Explanation      string      `json:"explanation,omitempty"`
	Plan             []PlanEntry `json:"plan,omitempty"`
	AwaitingApproval bool        `json:"awaiting_approval,omitempty"`
}

type GoalEvent struct {
	ThreadID        string    `json:"thread_id,omitempty"`
	Objective       string    `json:"objective,omitempty"`
	Status          string    `json:"status"`
	TokenBudget     *int64    `json:"token_budget,omitempty"`
	TokensUsed      int64     `json:"tokens_used,omitempty"`
	RemainingTokens *int64    `json:"remaining_tokens,omitempty"`
	TimeUsedSeconds int64     `json:"time_used_seconds,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitzero"`
	UpdatedAt       time.Time `json:"updated_at,omitzero"`
}

type ACPModeState struct {
	CurrentModeID  string    `json:"current_mode_id,omitempty"`
	PlanModeID     string    `json:"plan_mode_id,omitempty"`
	AvailableModes []ACPMode `json:"available_modes,omitempty"`
}

type ACPMode struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type PlanEntry struct {
	Content  string `json:"content"`
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
}

type ACPPlanEntry = PlanEntry

type ACPToolCall struct {
	ID        string           `json:"id"`
	Title     string           `json:"title,omitempty"`
	Status    string           `json:"status,omitempty"`
	Kind      string           `json:"kind,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
	Content   []ACPToolContent `json:"content,omitempty"`
	RawInput  map[string]any   `json:"raw_input,omitempty"`
	Runtime   ACPToolRuntime   `json:"runtime,omitzero"`
	StartedAt time.Time        `json:"started_at,omitzero"`
	UpdatedAt time.Time        `json:"updated_at,omitzero"`
}

// ACPToolContent is a normalized tool-call result block, distilled from the ACP
// ToolCallContent union so every agent (claude/codex/opencode/native) renders
// through one shape. Type is "text", "link", or "diff".
type ACPToolContent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	URI     string `json:"uri,omitempty"`
	Title   string `json:"title,omitempty"`
	Path    string `json:"path,omitempty"`
	OldText string `json:"old_text,omitempty"`
	NewText string `json:"new_text,omitempty"`
}

type ACPToolRuntime struct {
	TerminalID         string    `json:"terminal_id,omitempty"`
	TerminalCwd        string    `json:"terminal_cwd,omitempty"`
	ParentToolUseID    string    `json:"parent_tool_use_id,omitempty"`
	ElapsedTimeSeconds float64   `json:"elapsed_time_seconds,omitempty"`
	TerminalOutputAt   time.Time `json:"terminal_output_at,omitzero"`
	TerminalExitCode   *int      `json:"terminal_exit_code,omitempty"`
	TerminalExitSignal *string   `json:"terminal_exit_signal,omitempty"`
}

func (r ACPToolRuntime) IsZero() bool {
	return r.TerminalID == "" &&
		r.TerminalCwd == "" &&
		r.ParentToolUseID == "" &&
		r.ElapsedTimeSeconds == 0 &&
		r.TerminalOutputAt.IsZero() &&
		r.TerminalExitCode == nil &&
		r.TerminalExitSignal == nil
}

type ACPPermission struct {
	ID               string                  `json:"id"`
	SessionID        string                  `json:"session_id,omitempty"`
	Title            string                  `json:"title,omitempty"`
	ToolCallID       string                  `json:"tool_call_id,omitempty"`
	Content          string                  `json:"content,omitempty"`
	Options          []ACPPermissionOption   `json:"options,omitempty"`
	Locations        []ACPPermissionLocation `json:"locations,omitempty"`
	Questions        []ACPQuestion           `json:"questions,omitempty"`
	Status           string                  `json:"status,omitempty"`
	SelectedOptionID string                  `json:"selected_option_id,omitempty"`
}

type ACPQuestion struct {
	ID       string              `json:"id"`
	Header   string              `json:"header,omitempty"`
	Question string              `json:"question"`
	IsOther  bool                `json:"is_other,omitempty"`
	IsSecret bool                `json:"is_secret,omitempty"`
	Options  []ACPQuestionOption `json:"options,omitempty"`
}

type ACPQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type ACPPermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type ACPPermissionLocation struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

type Bus struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{}
}

const subscriberBuffer = 64

func New() *Bus {
	return &Bus{subs: map[string]map[chan Event]struct{}{}}
}

func (b *Bus) Subscribe(ctx context.Context, sessionID string) <-chan Event {
	ch := make(chan Event, subscriberBuffer)
	b.mu.Lock()
	if b.subs[sessionID] == nil {
		b.subs[sessionID] = map[chan Event]struct{}{}
	}
	b.subs[sessionID][ch] = struct{}{}
	b.mu.Unlock()
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		if subs := b.subs[sessionID]; subs != nil {
			if _, ok := subs[ch]; ok {
				delete(subs, ch)
				close(ch)
			}
			if len(subs) == 0 {
				delete(b.subs, sessionID)
			}
		}
		b.mu.Unlock()
	}()
	return ch
}

func (b *Bus) Publish(event Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	b.mu.Lock()
	subs := b.subs[event.SessionID]
	for ch := range subs {
		select {
		case ch <- event:
		default:
			delete(subs, ch)
			close(ch)
		}
	}
	if len(subs) == 0 {
		delete(b.subs, event.SessionID)
	}
	b.mu.Unlock()
}
