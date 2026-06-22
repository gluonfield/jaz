package acp

import (
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type Job struct {
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
	Plan          []sessionevents.PlanEntry     `json:"plan,omitempty"`
	ToolCalls     []sessionevents.ACPToolCall   `json:"tool_calls,omitempty"`
	Permissions   []sessionevents.ACPPermission `json:"permissions,omitempty"`
	Modes         ModeState                     `json:"modes,omitempty"`
	Error         string                        `json:"error,omitempty"`
	ParentVisible bool                          `json:"parent_visible,omitempty"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
	LastEventAt   time.Time                     `json:"last_event_at,omitzero"`
	LastToolAt    time.Time                     `json:"last_tool_at,omitzero"`

	mu                     sync.RWMutex
	turnMu                 sync.Mutex
	done                   chan struct{}
	completion             CompletionMode
	interactive            bool
	planRequested          bool
	cancelRequested        bool
	toolByID               map[string]sessionevents.ACPToolCall
	savedAssistantLen      int
	usage                  storage.Usage
	lastUsageDelta         storage.Usage
	lastUsageContext       storage.Usage
	lastUsageDeltaSet      bool
	systemPromptExtensions promptmodule.Modules
}

type ModeState struct {
	CurrentModeID  string         `json:"current_mode_id,omitempty"`
	PlanModeID     string         `json:"plan_mode_id,omitempty"`
	AvailableModes []ModeSnapshot `json:"available_modes,omitempty"`
}

type ModeSnapshot struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

func (j *Job) Snapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Job{
		ID:            j.ID,
		Slug:          j.Slug,
		Title:         j.Title,
		ParentID:      j.ParentID,
		ACPAgent:      j.ACPAgent,
		ACPSession:    j.ACPSession,
		Cwd:           j.Cwd,
		State:         j.State,
		StopReason:    j.StopReason,
		Assistant:     j.Assistant,
		Thought:       j.Thought,
		Plan:          clonePlanEntries(j.Plan),
		ToolCalls:     CloneToolCalls(j.ToolCalls),
		Permissions:   clonePermissions(j.Permissions),
		Modes:         j.Modes.Clone(),
		Error:         j.Error,
		ParentVisible: j.ParentVisible,
		CreatedAt:     j.CreatedAt,
		UpdatedAt:     j.UpdatedAt,
		LastEventAt:   j.LastEventAt,
		LastToolAt:    j.LastToolAt,
	}
}

func clonePlanEntries(in []sessionevents.PlanEntry) []sessionevents.PlanEntry {
	if in == nil {
		return nil
	}
	return append(make([]sessionevents.PlanEntry, 0, len(in)), in...)
}

func (s ModeState) Clone() ModeState {
	return ModeState{
		CurrentModeID:  s.CurrentModeID,
		PlanModeID:     s.PlanModeID,
		AvailableModes: append([]ModeSnapshot(nil), s.AvailableModes...),
	}
}

func (j *Job) setState(state, stopReason, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now().UTC()
	j.State = state
	j.StopReason = stopReason
	j.Error = errMsg
	j.UpdatedAt = now
	j.LastEventAt = now
}

func (j *Job) startTurn(completion CompletionMode, interactive, planRequested, parentVisible bool) chan struct{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = StateRunning
	j.Assistant = ""
	j.Thought = ""
	j.Plan = nil
	j.ToolCalls = nil
	j.Permissions = nil
	j.Error = ""
	j.StopReason = ""
	j.savedAssistantLen = 0
	j.usage = storage.Usage{}
	j.lastUsageDelta = storage.Usage{}
	j.lastUsageContext = storage.Usage{}
	j.lastUsageDeltaSet = false
	j.completion = completion
	j.interactive = interactive
	j.planRequested = planRequested
	j.cancelRequested = false
	j.ParentVisible = parentVisible
	j.toolByID = make(map[string]sessionevents.ACPToolCall)
	j.done = make(chan struct{})
	now := time.Now().UTC()
	j.UpdatedAt = now
	j.LastEventAt = now
	j.LastToolAt = time.Time{}
	return j.done
}

func CloneToolCalls(in []sessionevents.ACPToolCall) []sessionevents.ACPToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]sessionevents.ACPToolCall, 0, len(in))
	for _, call := range in {
		call.Content = append([]sessionevents.ACPToolContent(nil), call.Content...)
		call.RawInput = cloneMap(call.RawInput)
		call.Runtime = cloneToolRuntime(call.Runtime)
		out = append(out, call)
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneToolRuntime(in sessionevents.ACPToolRuntime) sessionevents.ACPToolRuntime {
	if in.TerminalExitCode != nil {
		code := *in.TerminalExitCode
		in.TerminalExitCode = &code
	}
	if in.TerminalExitSignal != nil {
		signal := *in.TerminalExitSignal
		in.TerminalExitSignal = &signal
	}
	return in
}

func clonePermissions(in []sessionevents.ACPPermission) []sessionevents.ACPPermission {
	if len(in) == 0 {
		return nil
	}
	out := make([]sessionevents.ACPPermission, 0, len(in))
	for _, permission := range in {
		permission.Options = append([]sessionevents.ACPPermissionOption(nil), permission.Options...)
		permission.Locations = append([]sessionevents.ACPPermissionLocation(nil), permission.Locations...)
		if len(permission.Questions) > 0 {
			questions := make([]sessionevents.ACPQuestion, 0, len(permission.Questions))
			for _, question := range permission.Questions {
				question.Options = append([]sessionevents.ACPQuestionOption(nil), question.Options...)
				questions = append(questions, question)
			}
			permission.Questions = questions
		}
		out = append(out, permission)
	}
	return out
}
