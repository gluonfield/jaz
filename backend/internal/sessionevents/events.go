package sessionevents

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	Seq        int64          `json:"seq,omitempty"`
	SessionID  string         `json:"session_id"`
	Type       string         `json:"type"`
	Content    string         `json:"content,omitempty"`
	ACP        *ACPEvent      `json:"acp,omitempty"`
	Plan       *PlanEvent     `json:"plan,omitempty"`
	Permission *ACPPermission `json:"permission,omitempty"`
	At         time.Time      `json:"at"`
}

type ACPEvent struct {
	ID          string          `json:"id"`
	Slug        string          `json:"slug"`
	Title       string          `json:"title,omitempty"`
	ParentID    string          `json:"parent_id,omitempty"`
	Agent       string          `json:"agent"`
	SessionID   string          `json:"session_id"`
	State       string          `json:"state"`
	StopReason  string          `json:"stop_reason,omitempty"`
	Assistant   string          `json:"assistant,omitempty"`
	Thought     string          `json:"thought,omitempty"`
	Error       string          `json:"error,omitempty"`
	Modes       ACPModeState    `json:"modes,omitempty"`
	Plan        []PlanEntry     `json:"plan,omitempty"`
	ToolCalls   []ACPToolCall   `json:"tool_calls,omitempty"`
	Permissions []ACPPermission `json:"permissions,omitempty"`
}

type PlanEvent struct {
	Explanation      string      `json:"explanation,omitempty"`
	Plan             []PlanEntry `json:"plan,omitempty"`
	AwaitingApproval bool        `json:"awaiting_approval,omitempty"`
}

type ACPModeState struct {
	CurrentModeID   string    `json:"current_mode_id,omitempty"`
	ExecutionModeID string    `json:"execution_mode_id,omitempty"`
	PlanModeID      string    `json:"plan_mode_id,omitempty"`
	AvailableModes  []ACPMode `json:"available_modes,omitempty"`
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
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

type ACPPermission struct {
	ID               string                  `json:"id"`
	SessionID        string                  `json:"session_id,omitempty"`
	Title            string                  `json:"title,omitempty"`
	ToolCallID       string                  `json:"tool_call_id,omitempty"`
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

func New() *Bus {
	return &Bus{subs: map[string]map[chan Event]struct{}{}}
}

func (b *Bus) Subscribe(ctx context.Context, sessionID string) <-chan Event {
	ch := make(chan Event, 16)
	b.mu.Lock()
	if b.subs[sessionID] == nil {
		b.subs[sessionID] = map[chan Event]struct{}{}
	}
	b.subs[sessionID][ch] = struct{}{}
	b.mu.Unlock()
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subs[sessionID], ch)
		if len(b.subs[sessionID]) == 0 {
			delete(b.subs, sessionID)
		}
		b.mu.Unlock()
		close(ch)
	}()
	return ch
}

func (b *Bus) Publish(event Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	b.mu.Lock()
	subs := make([]chan Event, 0, len(b.subs[event.SessionID]))
	for ch := range b.subs[event.SessionID] {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}
