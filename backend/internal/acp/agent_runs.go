package acp

import (
	"encoding/json"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

// The agent can begin work that no prompt asked for: Claude Code wakes its
// model when a background task finishes after the turn that started it ended.
// The adapter reports when the agent's work runs and ends, as
// `_meta.jaz.stateUpdate` (ACP v2's state_update carried on v1). Work that no
// turn holds runs as a turn of its own, owing its result to whoever waits for
// the background work it reports on.

// agentWork is what Jaz tracks of an agent that reports its work.
type agentWork struct {
	reports bool
	tasks   taskOwners
	// deferred holds the updates of a run that began while the previous turn
	// was ending, until that turn has finished (see startAgentRun).
	deferred  []json.RawMessage
	replaying bool
}

type workState struct {
	State      string             `json:"state"`
	StopReason string             `json:"stopReason"`
	Error      *jsonrpc.Error     `json:"error"`
	Meta       sessionFailureMeta `json:"_meta"`
	// Tasks are the background tasks whose end the work took in since the
	// agent's last report.
	Tasks []string `json:"tasks"`
}

// applyWorkState applies raw when it reports the agent's work, and reports
// whether it did.
func (m *Manager) applyWorkState(job *jobState, raw json.RawMessage) bool {
	var update struct {
		Kind string `json:"sessionUpdate"`
		Meta struct {
			Jaz struct {
				StateUpdate json.RawMessage `json:"stateUpdate"`
			} `json:"jaz"`
		} `json:"_meta"`
	}
	var state workState
	if json.Unmarshal(raw, &update) != nil || update.Kind != "session_info_update" ||
		update.Meta.Jaz.StateUpdate == nil || json.Unmarshal(update.Meta.Jaz.StateUpdate, &state) != nil {
		return false
	}
	job.mu.Lock()
	job.work.reports = true
	job.mu.Unlock()
	if state.State == "idle" {
		m.takeIn(job, state.Tasks)
		m.endWork(job, update.Meta.Jaz.StateUpdate, state)
	} else {
		m.startAgentRun(job, raw)
		m.takeIn(job, state.Tasks)
	}
	return true
}

// startAgentRun runs the agent's work as a turn when no turn holds it. Work
// reported while a turn runs is that turn's. Work reported after a turn's
// calls ended but before the turn finished waits, with every update after it,
// until the turn has finished (see replayDeferred).
func (m *Manager) startAgentRun(job *jobState, report json.RawMessage) {
	release, err := m.admit()
	if err != nil {
		return
	}
	defer release()
	closed := m.processClosed(job.ID)
	job.mu.Lock()
	if job.endingLocked() {
		// First in line: the updates already held back came after it.
		job.work.deferred = append([]json.RawMessage{report}, job.work.deferred...)
		job.mu.Unlock()
		return
	}
	if job.turn != nil || job.retired || closed == nil {
		job.mu.Unlock()
		return
	}
	// The run owes whoever waits for the tasks that finished while no work
	// ran before it names them, so a restart that loses its report, or a
	// report that names none, still delivers to them.
	owed := job.work.tasks.idle()
	// A turn that holds the thread first takes this work over.
	started, err := m.store.ClaimTurn(job.ID, owed)
	if err != nil || !started {
		job.mu.Unlock()
		if err != nil {
			m.log.Error("start agent turn", "session", job.ID, "error", err)
		}
		return
	}
	completion := CompletionInline
	if owed.NotifyParent {
		completion = CompletionAsync
	}
	turn := job.beginTurnLocked(completion, owed)
	turn.agentRun, turn.promptObserved, turn.unsent = true, true, 0
	turn.closeFirstPromptSent()
	job.mu.Unlock()
	m.log.Info("acp agent turn started", "session", job.ID, "agent", job.ACPAgent, "notify_parent", owed.NotifyParent)
	if err := m.publishACP(job.eventView()); err != nil {
		m.log.Error("persist agent turn", "session", job.ID, "error", err)
	}
	go m.watchAgentRun(job, turn.done, closed)
}

// endWork records the agent's report that its work in the running turn
// ended, which ends the agent's own run.
func (m *Manager) endWork(job *jobState, report json.RawMessage, state workState) {
	job.mu.Lock()
	job.work.tasks.workEnded()
	turn := job.turn
	if turn == nil {
		job.mu.Unlock()
		return
	}
	run := turn.agentRun
	steered := turn.promptCalls > 1
	turn.agentRun = false
	job.mu.Unlock()
	if !run {
		return
	}
	m.recordRawUsage(job, report)
	if state.StopReason == "error" && !steered {
		var err error = &jsonrpc.Error{Message: "agent run failed"}
		if state.Error != nil {
			err = state.Error
		}
		go m.failPromptCall(turn.done, job, err)
		return
	}
	go m.completePromptCall(turn.done, job, state.StopReason, state.Meta.terminal())
}

// watchAgentRun fails the agent's run when its connection closes first, as a
// prompt call fails then.
func (m *Manager) watchAgentRun(job *jobState, done chan struct{}, closed <-chan struct{}) {
	select {
	case <-done:
	case <-closed:
		job.mu.Lock()
		turn := job.turn
		running := turn != nil && turn.done == done && turn.agentRun
		if running {
			turn.agentRun = false
		}
		job.mu.Unlock()
		if running {
			m.failPromptCall(done, job, jsonrpc.ErrClosed)
		}
	}
}

func (m *Manager) processClosed(id string) <-chan struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if process := m.processes[id]; process != nil {
		return process.closed
	}
	return nil
}

// noteBackgroundWork records whom the agent's background task reports to:
// what the turn in which it started owes (see taskOwners).
func (m *Manager) noteBackgroundWork(job *jobState, task sessionevents.AgentTask) {
	job.mu.Lock()
	defer job.mu.Unlock()
	if !job.work.reports {
		return
	}
	if !taskLive(task) {
		job.work.tasks.end(task.ID, job.turn != nil)
		return
	}
	if job.turn == nil || job.work.tasks.has(task.ID) {
		return
	}
	if session, err := m.store.LoadSession(job.ID); err == nil && session.Turn != nil {
		job.work.tasks.start(task.ID, session.Turn.Owed())
	}
}

// takeIn has the running turn owe what each finished background task in ids
// owes: the turn's work took the task in. A turn that only plans or compacts
// reports to no one, so a task it took in stays waiting.
func (m *Manager) takeIn(job *jobState, ids []string) {
	if len(ids) == 0 {
		return
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	turn := job.turn
	if turn == nil || job.endingLocked() || turn.planRequested || job.ActiveOperation != "" {
		return
	}
	for _, owed := range job.work.tasks.take(ids) {
		if err := m.store.OweTurn(job.ID, owed); err != nil {
			m.log.Error("owe a finished task's report", "session", job.ID, "error", err)
			continue
		}
		job.ParentVisible = job.ParentVisible || owed.ParentVisible
		if owed.NotifyParent {
			turn.completion = CompletionAsync
		}
	}
}

// replayDeferred applies the updates that waited for a turn to finish, until
// none is left or another turn is ending.
func (m *Manager) replayDeferred(job *jobState) {
	for {
		job.mu.Lock()
		if job.work.replaying || job.work.deferred == nil || job.endingLocked() {
			job.mu.Unlock()
			return
		}
		if len(job.work.deferred) == 0 {
			job.work.deferred = nil
			job.mu.Unlock()
			return
		}
		raw := job.work.deferred[0]
		job.work.deferred = job.work.deferred[1:]
		job.work.replaying = true
		job.mu.Unlock()
		m.applyJobUpdate(job, raw)
		job.mu.Lock()
		job.work.replaying = false
		job.mu.Unlock()
	}
}

// deferUpdate holds raw back while earlier updates wait (see startAgentRun).
func (m *Manager) deferUpdate(job *jobState, raw json.RawMessage) bool {
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.work.deferred == nil {
		return false
	}
	job.work.deferred = append(job.work.deferred, raw)
	return true
}
