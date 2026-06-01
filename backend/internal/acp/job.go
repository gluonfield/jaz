package acp

import (
	"sync"
	"time"
)

type Job struct {
	ID         string             `json:"id"`
	Slug       string             `json:"slug"`
	Title      string             `json:"title,omitempty"`
	ParentID   string             `json:"parent_id,omitempty"`
	ACPAgent   string             `json:"acp_agent"`
	ACPSession string             `json:"acp_session"`
	Cwd        string             `json:"cwd,omitempty"`
	State      string             `json:"state"`
	StopReason string             `json:"stop_reason,omitempty"`
	Assistant  string             `json:"assistant,omitempty"`
	Thought    string             `json:"thought,omitempty"`
	Plan       []PlanEntry        `json:"plan,omitempty"`
	ToolCalls  []ToolCallSnapshot `json:"tool_calls,omitempty"`
	Error      string             `json:"error,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`

	mu                sync.RWMutex
	turnMu            sync.Mutex
	done              chan struct{}
	toolByID          map[string]ToolCallSnapshot
	savedAssistantLen int
}

type PlanEntry struct {
	Content  string `json:"content"`
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
}

type ToolCallSnapshot struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

func (j *Job) Snapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Job{
		ID:         j.ID,
		Slug:       j.Slug,
		Title:      j.Title,
		ParentID:   j.ParentID,
		ACPAgent:   j.ACPAgent,
		ACPSession: j.ACPSession,
		Cwd:        j.Cwd,
		State:      j.State,
		StopReason: j.StopReason,
		Assistant:  j.Assistant,
		Thought:    j.Thought,
		Plan:       append([]PlanEntry(nil), j.Plan...),
		ToolCalls:  append([]ToolCallSnapshot(nil), j.ToolCalls...),
		Error:      j.Error,
		CreatedAt:  j.CreatedAt,
		UpdatedAt:  j.UpdatedAt,
	}
}

func (j *Job) setState(state, stopReason, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = state
	j.StopReason = stopReason
	j.Error = errMsg
	j.UpdatedAt = time.Now().UTC()
}

func (j *Job) startTurn() chan struct{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = StateRunning
	j.Assistant = ""
	j.Thought = ""
	j.Plan = nil
	j.ToolCalls = nil
	j.Error = ""
	j.StopReason = ""
	j.savedAssistantLen = 0
	j.toolByID = make(map[string]ToolCallSnapshot)
	j.done = make(chan struct{})
	j.UpdatedAt = time.Now().UTC()
	return j.done
}
