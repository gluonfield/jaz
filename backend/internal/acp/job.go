package acp

import (
	"context"
	"strings"
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
	ToolCalls       []sessionevents.ACPToolCall   `json:"tool_calls,omitempty"`
	Permissions     []sessionevents.ACPPermission `json:"permissions,omitempty"`
	Modes           ModeState                     `json:"modes,omitempty"`
	Error           string                        `json:"error,omitempty"`
	GoalRequested   bool                          `json:"goal_requested,omitempty"`
	ActiveOperation string                        `json:"active_operation,omitempty"`
	ParentVisible   bool                          `json:"parent_visible,omitempty"`
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`
	LastEventAt     time.Time                     `json:"last_event_at,omitzero"`
	LastToolAt      time.Time                     `json:"last_tool_at,omitzero"`
	// Reply is the last message without a later tool call or an interim phase.
	Reply string `json:"-"`
}

type HydrationView struct {
	ID              string
	Slug            string
	Title           string
	ParentID        string
	ACPAgent        string
	ACPSession      string
	Cwd             string
	ModelProvider   string
	Model           string
	ReasoningEffort string
	State           string
	StopReason      string
	Plan            []sessionevents.PlanEntry
	Permissions     []sessionevents.ACPPermission
	Modes           ModeState
	Error           string
	GoalRequested   bool
	ActiveOperation string
	ParentVisible   bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastEventAt     time.Time
	LastToolAt      time.Time
}

type eventView struct {
	ID              string
	Slug            string
	Title           string
	ParentID        string
	ACPAgent        string
	ACPSession      string
	Cwd             string
	ModelProvider   string
	Model           string
	ReasoningEffort string
	State           string
	StopReason      string
	Plan            []sessionevents.PlanEntry
	Modes           ModeState
	Error           string
	GoalRequested   bool
	ParentVisible   bool
	SuggestedPrompt string
	LastEventAt     time.Time
	LastToolAt      time.Time
}

type StreamTool struct {
	ID    string
	Title string
}

type StreamView struct {
	State     string
	Error     string
	Assistant string
	Thought   string
	Tools     []StreamTool
}

type jobState struct {
	Job

	mu                     sync.RWMutex
	sendMu                 sync.Mutex
	turnMu                 sync.Mutex
	agentSession           sessionevents.AgentSession
	suggestedPrompt        string
	backgroundTasks        map[string]sessionevents.AgentTask
	backgroundWork         bool
	steerMethod            steerMethod
	turn                   *activeTurn
	finishing              chan struct{}
	toolByID               map[string]sessionevents.ACPToolCall
	pendingToolUpdateByID  map[string]sessionevents.ACPToolCall
	subagentToolIDs        map[string]struct{}
	deferredToolIDs        map[string]struct{}
	savedAssistantLen      int
	usage                  usageAccumulator
	turnResultDiscarded    bool
	systemPromptExtensions promptmodule.Modules
	assistantText          strings.Builder
	thoughtText            strings.Builder
	replyText              strings.Builder
	replyID                string
	// leases counts callers holding the process outside a turn; retired marks
	// a job whose process is being stopped as idle, which takes no more.
	leases  int
	retired bool
	work    agentWork
}

type activeTurn struct {
	done                  chan struct{}
	completion            CompletionMode
	planRequested         bool
	goalRequested         bool
	startedAt             time.Time
	cancelRequested       bool
	cancelReason          string
	firstPromptSent       chan struct{}
	firstPromptSentClosed bool
	promptHandoff         chan struct{}
	promptCalls           int
	allowSilence          bool
	grokInterjections     map[string]*grokInterjection
	grokStopReason        string
	cancel                context.CancelFunc
	// retries counts the continuations sent after provider failures; while
	// retrying, the turn waits to send the next one. providerRetried holds
	// the failure categories the provider retried since the turn last made
	// progress.
	retries         int
	retrying        bool
	providerRetried map[string]bool
	// unsent counts the turn's prompt calls not yet sent, and prompts holds
	// the ids of those sent whose responses the connection has not yet read.
	unsent  int
	prompts map[string]struct{}
	// agentRun marks a turn that the agent began on its own while that run
	// goes on: it ends with the agent's report, not a prompt response.
	agentRun bool
}

type ModeState struct {
	CurrentModeID  string         `json:"current_mode_id,omitempty"`
	PlanModeID     string         `json:"plan_mode_id,omitempty"`
	AvailableModes []ModeSnapshot `json:"available_modes,omitempty"`
	planConfigID   string
	userModeID     string
}

type ModeSnapshot struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

func jobFromSession(session storage.Session, agentName, acpSessionID, cwd, state string) Job {
	job := Job{
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
	if session.Turn != nil {
		job.ParentVisible = session.Turn.ParentVisible
	}
	return job
}

func newIdleJob(session storage.Session, agentName, acpSessionID, cwd string, modes ModeState) *jobState {
	job := &jobState{Job: jobFromSession(session, agentName, acpSessionID, cwd, StateIdle)}
	now := time.Now().UTC()
	job.Modes = modes
	job.UpdatedAt = now
	job.LastEventAt = now
	job.toolByID = make(map[string]sessionevents.ACPToolCall)
	job.pendingToolUpdateByID = make(map[string]sessionevents.ACPToolCall)
	job.turnResultDiscarded = true
	return job
}

func (j *jobState) Snapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	snapshot := j.hydrationViewLocked().Job()
	snapshot.Assistant = j.Assistant
	snapshot.Reply = j.Reply
	snapshot.Thought = j.Thought
	snapshot.ToolCalls = CloneToolCalls(j.ToolCalls)
	return snapshot
}

func (j *jobState) hydrationView() HydrationView {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.hydrationViewLocked()
}

func (j *jobState) hydrationViewLocked() HydrationView {
	view := HydrationViewFromJob(j.Job)
	view.GoalRequested = j.turn != nil && j.turn.goalRequested && (j.State == StateRunning || j.State == StateStarting)
	return view
}

func HydrationViewFromJob(job Job) HydrationView {
	return HydrationView{
		ID: job.ID, Slug: job.Slug, Title: job.Title, ParentID: job.ParentID,
		ACPAgent: job.ACPAgent, ACPSession: job.ACPSession, Cwd: job.Cwd,
		ModelProvider: job.ModelProvider, Model: job.Model, ReasoningEffort: job.ReasoningEffort,
		State: job.State, StopReason: job.StopReason, Plan: clonePlanEntries(job.Plan),
		Permissions: clonePermissions(job.Permissions), Modes: job.Modes.Clone(), Error: job.Error,
		GoalRequested: job.GoalRequested, ActiveOperation: job.ActiveOperation, ParentVisible: job.ParentVisible,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt, LastEventAt: job.LastEventAt, LastToolAt: job.LastToolAt,
	}
}

func (v HydrationView) Job() Job {
	return Job{
		ID: v.ID, Slug: v.Slug, Title: v.Title, ParentID: v.ParentID,
		ACPAgent: v.ACPAgent, ACPSession: v.ACPSession, Cwd: v.Cwd,
		ModelProvider: v.ModelProvider, Model: v.Model, ReasoningEffort: v.ReasoningEffort,
		State: v.State, StopReason: v.StopReason, Plan: clonePlanEntries(v.Plan), Permissions: clonePermissions(v.Permissions),
		Modes: v.Modes.Clone(), Error: v.Error, GoalRequested: v.GoalRequested,
		ActiveOperation: v.ActiveOperation, ParentVisible: v.ParentVisible,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, LastEventAt: v.LastEventAt, LastToolAt: v.LastToolAt,
	}
}

func eventViewFromJob(job Job) eventView {
	return eventView{
		ID:              job.ID,
		Slug:            job.Slug,
		Title:           job.Title,
		ParentID:        job.ParentID,
		ACPAgent:        job.ACPAgent,
		ACPSession:      job.ACPSession,
		Cwd:             job.Cwd,
		ModelProvider:   job.ModelProvider,
		Model:           job.Model,
		ReasoningEffort: job.ReasoningEffort,
		State:           job.State,
		StopReason:      job.StopReason,
		Plan:            clonePlanEntries(job.Plan),
		Modes:           job.Modes.Clone(),
		Error:           job.Error,
		GoalRequested:   job.GoalRequested,
		ParentVisible:   job.ParentVisible,
		LastEventAt:     job.LastEventAt,
		LastToolAt:      job.LastToolAt,
	}
}

func (j *jobState) eventView() eventView {
	j.mu.RLock()
	defer j.mu.RUnlock()
	view := eventViewFromJob(j.Job)
	view.GoalRequested = j.turn != nil && j.turn.goalRequested && (j.State == StateRunning || j.State == StateStarting)
	view.SuggestedPrompt = j.suggestedPrompt
	return view
}

func (j *jobState) streamView() StreamView {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return StreamViewFromJob(j.Job)
}

func StreamViewFromJob(job Job) StreamView {
	view := StreamView{
		State:     job.State,
		Error:     job.Error,
		Assistant: job.Assistant,
		Thought:   job.Thought,
		Tools:     make([]StreamTool, 0, len(job.ToolCalls)),
	}
	for _, call := range job.ToolCalls {
		view.Tools = append(view.Tools, StreamTool{ID: call.ID, Title: call.Title})
	}
	return view
}

func clonePlanEntries(in []sessionevents.PlanEntry) []sessionevents.PlanEntry {
	if in == nil {
		return nil
	}
	return append(make([]sessionevents.PlanEntry, 0, len(in)), in...)
}

func (s ModeState) Clone() ModeState {
	s.AvailableModes = append([]ModeSnapshot(nil), s.AvailableModes...)
	return s
}

func (j *jobState) setState(state, stopReason, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now().UTC()
	j.State = state
	j.StopReason = stopReason
	j.Error = errMsg
	if state != StateRunning && state != StateStarting {
		j.ActiveOperation = ""
	}
	j.UpdatedAt = now
	j.LastEventAt = now
}

func (j *jobState) startTurn(completion CompletionMode, planRequested, parentVisible bool) chan struct{} {
	return j.startStoredTurn(completion, storage.Turn{PlanRequested: planRequested, ParentVisible: parentVisible})
}

func (j *jobState) startStoredTurn(completion CompletionMode, stored storage.Turn) chan struct{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.beginTurnLocked(completion, stored).done
}

func (j *jobState) beginTurnLocked(completion CompletionMode, stored storage.Turn) *activeTurn {
	now := time.Now().UTC()
	j.State = StateRunning
	j.Assistant = ""
	j.Thought = ""
	j.assistantText.Reset()
	j.thoughtText.Reset()
	j.endReplyLocked()
	if len(j.Plan) > 0 {
		j.Plan = sessionevents.PlanCleared
	} else {
		j.Plan = nil
	}
	j.ToolCalls = nil
	j.Permissions = nil
	j.suggestedPrompt = ""
	j.Error = ""
	j.StopReason = ""
	j.ActiveOperation = stored.ActiveOperation
	j.savedAssistantLen = 0
	j.usage.startTurn()
	j.turnResultDiscarded = false
	j.turn = &activeTurn{
		done:            make(chan struct{}),
		completion:      completion,
		planRequested:   stored.PlanRequested,
		startedAt:       now,
		firstPromptSent: make(chan struct{}),
		promptCalls:     1,
		allowSilence:    stored.AllowSilence,
		goalRequested:   stored.GoalRequested,
		retries:         stored.Retries,
		unsent:          1,
		prompts:         map[string]struct{}{},
	}
	j.ParentVisible = stored.ParentVisible
	j.toolByID = make(map[string]sessionevents.ACPToolCall)
	j.pendingToolUpdateByID = make(map[string]sessionevents.ACPToolCall)
	j.UpdatedAt = now
	j.LastEventAt = now
	j.LastToolAt = time.Time{}
	return j.turn
}

// promptSending records that the running turn sends its prompt call id.
func (j *jobState) promptSending(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn != nil {
		j.turn.unsent--
		j.turn.prompts[id] = struct{}{}
	}
}

// promptAnswered records the response to prompt call id as the connection
// reads it, before the turn learns of it.
func (j *jobState) promptAnswered(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn != nil {
		delete(j.turn.prompts, id)
	}
}

// endingLocked reports whether the job's turn is ending: every call it made
// has been answered, and the turn has yet to finish. Work the agent reports
// before then is the turn's: the Claude adapter holds a turn's answer until
// the background subagents it started have reported.
func (j *jobState) endingLocked() bool {
	return j.finishing != nil || j.turn != nil && !j.turn.agentRun && j.turn.unsent == 0 && len(j.turn.prompts) == 0
}

func (j *jobState) discardTurnResult() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn != nil || j.State == StateRunning || j.State == StateStarting {
		return
	}
	j.Assistant = ""
	j.Thought = ""
	j.Plan = nil
	j.ToolCalls = nil
	j.Permissions = nil
	j.ActiveOperation = ""
	j.assistantText.Reset()
	j.thoughtText.Reset()
	j.endReplyLocked()
	j.toolByID = nil
	j.pendingToolUpdateByID = nil
	j.turnResultDiscarded = true
}

func (j *jobState) resultDiscarded() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.turnResultDiscarded
}

func (j *jobState) appendAssistantLocked(chunk string) {
	if j.assistantText.Len() == 0 && j.Assistant != "" {
		j.assistantText.WriteString(j.Assistant)
	}
	j.assistantText.WriteString(chunk)
	j.Assistant = j.assistantText.String()
}

// replyLocked adds a chunk of message id to the turn's final reply, which
// another message replaces.
func (j *jobState) replyLocked(id, chunk string) {
	if id != j.replyID {
		j.endReplyLocked()
		j.replyID = id
	}
	j.replyText.WriteString(chunk)
	j.Reply = j.replyText.String()
}

// endReplyLocked clears the final reply as the agent acts after writing it.
func (j *jobState) endReplyLocked() {
	j.replyText.Reset()
	j.Reply = ""
}

func (j *jobState) appendThoughtLocked(chunk string) {
	if j.thoughtText.Len() == 0 && j.Thought != "" {
		j.thoughtText.WriteString(j.Thought)
	}
	j.thoughtText.WriteString(chunk)
	j.Thought = j.thoughtText.String()
}

func (j *jobState) turnDone() chan struct{} {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.turn == nil {
		return j.finishing
	}
	return j.turn.done
}

func (j *jobState) turnDoneAndPlan() (chan struct{}, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.turn == nil {
		return nil, false
	}
	return j.turn.done, j.turn.planRequested
}

func (j *jobState) requestCancel() (bool, chan struct{}) {
	return j.requestTurnCancel(StopReasonCancelled)
}

func (j *jobState) requestShutdown() (bool, chan struct{}) {
	return j.requestTurnCancel(StopReasonServerShutdown)
}

func (j *jobState) requestTurnCancel(reason string) (bool, chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	running := j.State == StateRunning || j.State == StateStarting
	if j.turn == nil {
		return running, j.finishing
	}
	if reason == StopReasonServerShutdown && j.turn.cancelRequested {
		return false, j.turn.done
	}
	if running {
		j.turn.cancelRequested = true
		j.turn.cancelReason = reason
	}
	return running, j.turn.done
}

func (j *jobState) cancelReason() (string, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.turn == nil || !j.turn.cancelRequested {
		return "", false
	}
	if j.turn.cancelReason != "" {
		return j.turn.cancelReason, true
	}
	return StopReasonCancelled, true
}

func (j *jobState) setTurnCancel(done chan struct{}, cancel context.CancelFunc) bool {
	j.mu.Lock()
	if j.turn == nil || j.turn.done != done {
		j.mu.Unlock()
		return false
	}
	j.turn.cancel = cancel
	requested := j.turn.cancelRequested
	j.mu.Unlock()
	if requested {
		cancel()
	}
	return true
}

func (j *jobState) clearTurnCancel(done chan struct{}) {
	j.mu.Lock()
	if j.turn != nil && j.turn.done == done {
		j.turn.cancel = nil
	}
	j.mu.Unlock()
}

func (j *jobState) turnCancel() context.CancelFunc {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.turn == nil {
		return nil
	}
	return j.turn.cancel
}

func (j *jobState) hasQueuedPromptSuccessor() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.steerMethod == steerPromptQueueing && j.turn != nil && j.turn.promptCalls > 1
}

func (j *jobState) requirePromptHandoff(done chan struct{}) <-chan struct{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn == nil || j.turn.done != done {
		return nil
	}
	if j.turn.promptHandoff == nil {
		j.turn.promptHandoff = make(chan struct{})
	}
	return j.turn.promptHandoff
}

func (j *jobState) currentPromptHandoff(done chan struct{}) <-chan struct{} {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.turn == nil || j.turn.done != done {
		return nil
	}
	return j.turn.promptHandoff
}

func (j *jobState) waitFirstPromptSent(ctx context.Context) error {
	j.mu.RLock()
	turn := j.turn
	if turn == nil || turn.firstPromptSentClosed {
		j.mu.RUnlock()
		return nil
	}
	ch := turn.firstPromptSent
	j.mu.RUnlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (j *jobState) markFirstPromptSent() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn != nil {
		j.turn.closeFirstPromptSent()
	}
}

// waitToRetry holds the turn for one more prompt call after delay: the call
// counts as outstanding so senders see the turn running, steering waits for
// it to reach the agent, and Stop or shutdown end the wait through the
// turn's cancel.
func (t *activeTurn) waitToRetry(delay time.Duration) context.Context {
	t.retries++
	t.retrying = true
	t.promptCalls = 1
	t.unsent++
	t.firstPromptSent, t.firstPromptSentClosed = make(chan struct{}), false
	wait, cancel := context.WithTimeout(context.Background(), delay)
	t.cancel = cancel
	return wait
}

// endRetryWait ends the turn's retry wait and reports whether the turn was
// stopped meanwhile. Steering stays closed until the continuation is sent.
func (t *activeTurn) endRetryWait() bool {
	t.cancel = nil
	t.retrying = false
	return t.cancelRequested
}

func (t *activeTurn) closeFirstPromptSent() {
	if !t.firstPromptSentClosed {
		close(t.firstPromptSent)
		t.firstPromptSentClosed = true
	}
}

func CloneToolCalls(in []sessionevents.ACPToolCall) []sessionevents.ACPToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]sessionevents.ACPToolCall, 0, len(in))
	for _, call := range in {
		call.Content = append([]sessionevents.ACPToolContent(nil), call.Content...)
		call.Locations = append([]sessionevents.ACPToolLocation(nil), call.Locations...)
		call.RawInput = cloneMap(call.RawInput)
		call.RawOutput = append([]byte(nil), call.RawOutput...)
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
