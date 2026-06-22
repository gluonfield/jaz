package acp

import (
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type Job struct {
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
	Plan            []sessionevents.PlanEntry     `json:"plan,omitempty"`
	ToolCalls       []ToolCallSnapshot            `json:"tool_calls,omitempty"`
	Permissions     []sessionevents.ACPPermission `json:"permissions,omitempty"`
	Modes           ModeState                     `json:"modes,omitempty"`
	Error           string                        `json:"error,omitempty"`
	ParentVisible   bool                          `json:"parent_visible,omitempty"`
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`

	mu                     sync.RWMutex
	turnMu                 sync.Mutex
	promptQueueing         bool
	turn                   *activeTurn
	toolByID               map[string]ToolCallSnapshot
	savedAssistantLen      int
	usage                  storage.Usage
	lastUsageDelta         storage.Usage
	lastUsageContext       storage.Usage
	lastUsageDeltaSet      bool
	systemPromptExtensions promptmodule.Modules
}

type activeTurn struct {
	done            chan struct{}
	completion      CompletionMode
	planRequested   bool
	cancelRequested bool
	promptCalls     int
}

type ToolCallSnapshot struct {
	ID       string                         `json:"id"`
	Title    string                         `json:"title,omitempty"`
	Status   string                         `json:"status,omitempty"`
	Kind     string                         `json:"kind,omitempty"`
	ToolName string                         `json:"tool_name,omitempty"`
	Content  []sessionevents.ACPToolContent `json:"content,omitempty"`
	RawInput map[string]any                 `json:"raw_input,omitempty"`
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

func jobFromSession(session storage.Session, agentName, acpSessionID, cwd, state string) Job {
	return Job{
		ID:              session.ID,
		Slug:            session.Slug,
		Title:           session.Title,
		ParentID:        session.ParentID,
		ACPAgent:        CanonicalAgentName(agentName),
		ACPSession:      acpSessionID,
		Cwd:             cwd,
		ModelProvider:   session.ModelProvider,
		Model:           session.Model,
		ReasoningEffort: session.ReasoningEffort,
		State:           state,
		CreatedAt:       session.CreatedAt,
		UpdatedAt:       session.UpdatedAt,
	}
}

func newIdleJob(session storage.Session, agentName, acpSessionID, cwd string, modes ModeState) *Job {
	job := jobFromSession(session, agentName, acpSessionID, cwd, StateIdle)
	job.Modes = modes
	job.UpdatedAt = time.Now().UTC()
	job.toolByID = make(map[string]ToolCallSnapshot)
	return &job
}

func (j *Job) Snapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Job{
		ID:              j.ID,
		Slug:            j.Slug,
		Title:           j.Title,
		ParentID:        j.ParentID,
		ACPAgent:        j.ACPAgent,
		ACPSession:      j.ACPSession,
		Cwd:             j.Cwd,
		ModelProvider:   j.ModelProvider,
		Model:           j.Model,
		ReasoningEffort: j.ReasoningEffort,
		State:           j.State,
		StopReason:      j.StopReason,
		Assistant:       j.Assistant,
		Thought:         j.Thought,
		Plan:            clonePlanEntries(j.Plan),
		ToolCalls:       append([]ToolCallSnapshot(nil), j.ToolCalls...),
		Permissions:     clonePermissions(j.Permissions),
		Modes:           j.Modes.Clone(),
		Error:           j.Error,
		ParentVisible:   j.ParentVisible,
		CreatedAt:       j.CreatedAt,
		UpdatedAt:       j.UpdatedAt,
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
	j.State = state
	j.StopReason = stopReason
	j.Error = errMsg
	j.UpdatedAt = time.Now().UTC()
}

func (j *Job) startTurn(completion CompletionMode, planRequested, parentVisible bool) chan struct{} {
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
	j.turn = &activeTurn{
		done:          make(chan struct{}),
		completion:    completion,
		planRequested: planRequested,
		promptCalls:   1,
	}
	j.ParentVisible = parentVisible
	j.toolByID = make(map[string]ToolCallSnapshot)
	j.UpdatedAt = time.Now().UTC()
	return j.turn.done
}

func (j *Job) turnDone() chan struct{} {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.turn == nil {
		return nil
	}
	return j.turn.done
}

func (j *Job) turnDoneAndPlan() (chan struct{}, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.turn == nil {
		return nil, false
	}
	return j.turn.done, j.turn.planRequested
}

func (j *Job) requestCancel() (bool, chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn != nil {
		j.turn.cancelRequested = true
	}
	running := j.State == StateRunning || j.State == StateStarting
	if j.turn == nil {
		return running, nil
	}
	return running, j.turn.done
}

func (j *Job) addPromptCall(parentVisible bool) (chan struct{}, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.promptQueueing || j.turn == nil {
		return nil, false
	}
	if j.State != StateRunning && j.State != StateStarting {
		return nil, false
	}
	if parentVisible {
		j.ParentVisible = true
	}
	j.turn.promptCalls++
	return j.turn.done, true
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
