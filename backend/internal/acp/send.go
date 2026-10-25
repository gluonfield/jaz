package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessiongoal"
	"github.com/wins/jaz/backend/internal/storage"
)

var ErrSteeringUnsupported = errors.New("acp steering unsupported")

var errTurnEnded = fmt.Errorf("%w: agent turn ended before steering", ErrSteeringUnsupported)

// errPromptPending refuses a steer while the turn's next prompt has not
// reached the agent, as while it waits to retry a provider failure.
var errPromptPending = fmt.Errorf("%w: the turn's next prompt has not reached the agent", ErrSteeringUnsupported)

var errQuestionPending = errors.New("the turn is waiting for an answer")

type turnInProgressError struct {
	done      <-chan struct{}
	finishing bool
}

func (e *turnInProgressError) Error() string {
	return "session is already running"
}

// sendConflict refuses a turn while another runs or ends, and while a new
// thread's agent waits for the turn it was created with unless this send
// starts it.
func (j *jobState) sendConflict(claimed bool) *turnInProgressError {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.sendConflictLocked(claimed)
}

func (j *jobState) sendConflictLocked(claimed bool) *turnInProgressError {
	if j.turn != nil {
		return &turnInProgressError{done: j.turn.done, finishing: j.turn.promptCalls == 0 || (j.State != StateRunning && j.State != StateStarting)}
	}
	if j.finishing != nil {
		return &turnInProgressError{done: j.finishing, finishing: true}
	}
	if j.State == StateStarting && !claimed {
		return &turnInProgressError{}
	}
	return nil
}

type sendTranscriptMode int

const (
	sendTranscriptUserMessage sendTranscriptMode = iota
	sendTranscriptHidden
)

type sendOptions struct {
	activeOperation       string
	transcript            sendTranscriptMode
	requireCompactSupport bool
	requireActiveGoal     bool
	allowSilence          bool
	output                *storage.TurnOutput
	owes                  []storage.Recipient
	retries               int
	continuationOf        time.Time
	// claimed starts the turn a new thread was created with.
	claimed bool
}

type InternalTurnRequest struct {
	Session string
	Message string
	Output  *storage.TurnOutput
	// AllowSilence lets the turn end without a message or tool call, for
	// turns whose agent may rightly have nothing to say.
	AllowSilence bool
}

func (m *Manager) Send(ctx context.Context, req SendRequest) (Job, error) {
	return m.send(ctx, req, sendOptions{transcript: sendTranscriptUserMessage})
}

// ContinueGoal starts an automatic follow-up turn for a still-active goal.
func (m *Manager) ContinueGoal(ctx context.Context, session string) (Job, error) {
	return m.send(ctx, SendRequest{
		Session:       session,
		Message:       jazGoalContinuationMessage,
		Completion:    CompletionAsync,
		GoalRequested: true,
	}, sendOptions{transcript: sendTranscriptHidden, requireActiveGoal: true})
}

func (m *Manager) StartInternalTurn(ctx context.Context, req InternalTurnRequest) (Job, error) {
	return m.send(ctx, SendRequest{
		Session:    req.Session,
		Message:    req.Message,
		Completion: CompletionAsync,
	}, sendOptions{transcript: sendTranscriptHidden, allowSilence: req.AllowSilence, output: req.Output})
}

func (m *Manager) Compact(ctx context.Context, req CompactRequest) (Job, error) {
	return m.send(ctx, SendRequest{
		Session:    req.Session,
		Message:    CompactCommand,
		Completion: CompletionInline,
	}, sendOptions{
		activeOperation:       ActiveOperationCompact,
		transcript:            sendTranscriptHidden,
		requireCompactSupport: true,
	})
}

func (m *Manager) send(ctx context.Context, req SendRequest, opts sendOptions) (Job, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Job{}, err
		}
		job, err := m.sendOnce(ctx, req, opts)
		var active *turnInProgressError
		if !errors.As(err, &active) || !active.finishing {
			return job, err
		}
		select {
		case <-active.done:
		case <-ctx.Done():
			return Job{}, ctx.Err()
		}
	}
}

func (m *Manager) sendOnce(ctx context.Context, req SendRequest, opts sendOptions) (Job, error) {
	if err := m.store.CheckTaskAdmission(req.Session); err != nil {
		return Job{}, err
	}
	if !storage.HasMessageContent(req.Message, req.Contexts, req.Attachments) {
		return Job{}, fmt.Errorf("message is required")
	}
	job, err := m.job(req.Session)
	if err != nil {
		job, err = m.resume(ctx, req.Session)
	}
	if err != nil {
		return Job{}, err
	}
	local := m.localAgent(job.ACPAgent)
	if m.configuredLocal(job.ACPAgent) && local == nil {
		return Job{}, fmt.Errorf("local acp agent %q is not registered", job.ACPAgent)
	}
	job.sendMu.Lock()
	if err := job.sendConflict(opts.claimed); err != nil {
		job.sendMu.Unlock()
		return Job{}, fmt.Errorf("%s: %w", job.Slug, err)
	}
	job.sendMu.Unlock()
	if local == nil {
		job, err = m.acquireSessionProcess(ctx, job)
		if err != nil {
			return Job{}, err
		}
		defer job.release()
	}
	if opts.requireCompactSupport && !m.supportsSessionCommand(job, "compact") {
		return Job{}, fmt.Errorf("compact is not available for acp agent %q", job.ACPAgent)
	}
	job.sendMu.Lock()
	defer job.sendMu.Unlock()
	if err := job.sendConflict(opts.claimed); err != nil {
		return Job{}, fmt.Errorf("%s: %w", job.Slug, err)
	}
	if opts.requireActiveGoal {
		session, err := m.store.LoadSession(job.ID)
		if err != nil {
			return Job{}, err
		}
		if !goal.Continuable(session.Goal) {
			return Job{}, sessiongoal.ErrNoActiveGoal
		}
	}
	if err := m.refreshMCPBeforeTurn(ctx, job); err != nil {
		return Job{}, err
	}
	if err := m.prepareModeForTurn(ctx, job, req.PlanRequested); err != nil {
		return Job{}, err
	}
	release, err := m.admit()
	if err != nil {
		return Job{}, err
	}
	defer release()
	promptMessage, contexts := promptMessageAndContexts(req.Message, req.Contexts)
	pending := &storage.TurnPrompt{Message: promptMessage, Attachments: req.Attachments}
	var promptDigest string
	if m.cfg.TurnNote != nil && opts.activeOperation != ActiveOperationCompact {
		session, err := m.store.LoadSession(job.ID)
		if err != nil {
			return Job{}, err
		}
		var note string
		note, promptDigest = m.cfg.TurnNote(session)
		promptMessage = note + promptMessage
	}
	notifyParent := req.ParentVisible && req.Completion.propagates() && !req.PlanRequested
	// Work a parent starts while answering a bot reports back to that bot.
	parentOutput := req.ParentOutput
	if notifyParent && req.From != "" && req.From == job.ParentID {
		if parentOutput, err = m.answering(req.From); err != nil {
			return Job{}, err
		}
	}
	var prompt acpschema.PromptRequest
	if local == nil {
		prompt, err = m.promptRequest(job, goalPromptMessage(promptMessage, req.GoalRequested), req.Attachments)
		if err != nil {
			return Job{}, err
		}
	}
	output := opts.output
	if opts.transcript == sendTranscriptUserMessage && req.FromUser {
		output = &storage.TurnOutput{User: true}
	}
	turn := storage.Turn{
		StartedAt:      time.Now().UTC(),
		ContinuationOf: opts.continuationOf,
		PlanRequested:  req.PlanRequested, GoalRequested: req.GoalRequested, ActiveOperation: opts.activeOperation,
		ParentVisible: req.ParentVisible, NotifyParent: notifyParent,
		AllowSilence: opts.allowSilence, Output: output, ParentOutput: parentOutput,
		PromptDigest: promptDigest, Retries: opts.retries, Prompt: pending, Owes: opts.owes,
	}
	var messages []storage.Message
	if opts.transcript == sendTranscriptUserMessage {
		message := storage.UserMessageRecord(req.Message, contexts, req.Attachments)
		message.FromUser = req.FromUser
		messages = append(messages, message)
	}
	done, err := m.claimTurn(job, turn, req.Completion, opts.claimed, messages)
	if err != nil {
		return Job{}, fmt.Errorf("%s: %w", job.Slug, err)
	}
	m.log.Info("acp turn started", "session", job.ID, "agent", job.ACPAgent, "plan", req.PlanRequested, "goal", req.GoalRequested, "operation", opts.activeOperation)
	m.touchAttention(parentSessionIDs(job.eventView())...)
	if err := m.publishACP(job.eventView()); err != nil {
		go m.failPromptCall(done, job, fmt.Errorf("persist initial session events: %w", err))
	} else if local != nil {
		go m.runLocalPrompt(context.WithoutCancel(ctx), job, local, promptMessage, req.Attachments)
	} else {
		go m.runPrompt(context.Background(), job, prompt)
	}
	return job.Snapshot(), nil
}

// claimTurn starts a send's turn, with the user's messages, unless another
// turn holds the thread. It stores the turn under the job's lock, so the agent
// cannot begin a run of its own in between (see startAgentRun).
func (m *Manager) claimTurn(job *jobState, turn storage.Turn, completion CompletionMode, claimed bool, messages []storage.Message) (chan struct{}, error) {
	job.mu.Lock()
	defer job.mu.Unlock()
	if conflict := job.sendConflictLocked(claimed); conflict != nil {
		return nil, conflict
	}
	if err := m.store.StartSessionTurn(job.ID, turn, messages...); err != nil {
		return nil, fmt.Errorf("start session turn: %w", err)
	}
	return job.beginTurnLocked(completion, turn).done, nil
}

// SteerInternal hands a hidden message to the thread's running turn, as
// StartInternalTurn starts a hidden turn. A user's steer supersedes the
// questions the turn is waiting on; a hidden message must not, so it is
// refused while one is pending.
func (m *Manager) SteerInternal(ctx context.Context, session, message string) (Job, error) {
	return m.Steer(ctx, SteerRequest{Session: session, Message: message, transcript: sendTranscriptHidden})
}

func (m *Manager) Steer(ctx context.Context, req SteerRequest) (Job, error) {
	job, err := m.job(req.Session)
	if err != nil {
		return Job{}, fmt.Errorf("%w: %v", ErrSteeringUnsupported, err)
	}
	if !storage.HasMessageContent(req.Message, req.Contexts, req.Attachments) {
		return Job{}, fmt.Errorf("message is required")
	}
	job.mu.RLock()
	method := job.steerMethod
	running := job.turn != nil && (job.State == StateRunning || job.State == StateStarting)
	planRequested := job.turn != nil && job.turn.planRequested
	retrying := running && job.turn.retrying
	asking := len(job.Permissions) > 0
	job.mu.RUnlock()
	if !running {
		return Job{}, errTurnEnded
	}
	if req.PlanRequested && !planRequested {
		return Job{}, fmt.Errorf("cannot switch a running turn to plan mode")
	}
	if retrying {
		return Job{}, errPromptPending
	}
	if asking && req.transcript == sendTranscriptHidden {
		return Job{}, errQuestionPending
	}
	if method == steerUnsupported {
		return Job{}, ErrSteeringUnsupported
	}
	local := m.localAgent(job.ACPAgent)
	if m.configuredLocal(job.ACPAgent) && local == nil {
		return Job{}, fmt.Errorf("local acp agent %q is not registered", job.ACPAgent)
	}
	if local != nil {
		return Job{}, ErrSteeringUnsupported
	}
	if err := job.waitFirstPromptSent(ctx); err != nil {
		return Job{}, err
	}
	peer := m.peer(job.ID)
	if peer == nil {
		return Job{}, fmt.Errorf("acp peer is not active")
	}
	prepared, contexts := promptMessageAndContexts(req.Message, req.Contexts)
	note := ""
	if req.FromUser && m.cfg.SteerNote != nil {
		session, err := m.store.LoadSession(job.ID)
		if err != nil {
			return Job{}, err
		}
		note = m.cfg.SteerNote(session)
	}
	promptReq, err := m.promptRequest(job, goalPromptMessage(note+prepared, req.GoalRequested), req.Attachments)
	if err != nil {
		return Job{}, err
	}
	slot, err := m.reserveSteer(job, req, contexts, prepared)
	if err != nil {
		return Job{}, err
	}
	var handoff <-chan struct{}
	if req.transcript == sendTranscriptUserMessage {
		handoff = m.cancelPendingPermissionsForSteer(job, slot.done)
		m.touchJobAttention(job)
	} else {
		handoff = job.currentPromptHandoff(slot.done)
	}
	m.publishACP(job.eventView())
	go m.runSteerCallAfterHandoff(context.Background(), job, slot, handoff, method, promptReq)
	return job.Snapshot(), nil
}

// steerSlot is a steer's place in its turn. Steers go out in the order they
// were made: each waits until the one before it is on the wire.
type steerSlot struct {
	done  chan struct{}
	after <-chan struct{}
	sent  chan struct{}
}

func (m *Manager) reserveSteer(job *jobState, req SteerRequest, contexts []storage.MessageContext, prepared string) (steerSlot, error) {
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.turn == nil || job.turn.promptCalls == 0 || (job.State != StateRunning && job.State != StateStarting) {
		return steerSlot{}, errTurnEnded
	}
	// The gate is closed once the turn's latest prompt reached the agent; a
	// retry reopens it until its continuation does.
	if job.turn.retrying || !job.turn.firstPromptSentClosed {
		return steerSlot{}, errPromptPending
	}
	if job.steerMethod == steerUnsupported {
		return steerSlot{}, ErrSteeringUnsupported
	}
	session, err := m.store.LoadSession(job.ID)
	if err != nil {
		return steerSlot{}, err
	}
	if session.Turn == nil {
		return steerSlot{}, fmt.Errorf("active turn is missing from storage")
	}
	update := storage.TurnUpdate{
		GoalRequested: session.Turn.GoalRequested || req.GoalRequested,
		ParentVisible: session.Turn.ParentVisible || req.ParentVisible,
		EndExchange:   req.FromUser,
	}
	update.NotifyParent = update.ParentVisible && job.turn.completion.propagates() && !session.Turn.PlanRequested
	queued := job.steerMethod == steerPromptQueueing
	// A queued prompt waits for the agent to take it in, and a restart before
	// then would lose it, so the turn keeps it until then. The agent's own
	// run reports no take-in, so a steer into it is not kept.
	if queued && !job.turn.agentRun {
		update.Steer = &storage.TurnPrompt{Message: prepared, Attachments: req.Attachments}
	}
	var messages []storage.Message
	if req.transcript == sendTranscriptUserMessage {
		message := storage.UserMessageRecord(req.Message, contexts, req.Attachments)
		message.FromUser = req.FromUser
		messages = append(messages, message)
	}
	if err := m.store.UpdateTurn(job.ID, update, messages...); err != nil {
		return steerSlot{}, fmt.Errorf("reserve steering: %w", err)
	}
	job.turn.goalRequested = update.GoalRequested
	if req.ParentVisible {
		job.ParentVisible = true
	}
	job.turn.promptCalls++
	if queued {
		job.turn.unsent++
	}
	if update.Steer != nil {
		job.turn.kept++
	}
	slot := steerSlot{done: job.turn.done, after: job.turn.steerSent, sent: make(chan struct{})}
	job.turn.steerSent = slot.sent
	return slot, nil
}

func (m *Manager) runNativeSteerCall(ctx context.Context, job *jobState, done chan struct{}, prompt acpschema.PromptRequest) {
	peer := m.peer(job.ID)
	if peer == nil {
		m.failPromptCall(done, job, fmt.Errorf("acp peer is not active"))
		return
	}
	m.withACPTranscriptBarrier(job.eventView(), nil)
	raw, err := peer.Call(ctx, string(steerNative), struct {
		SessionID         acpschema.SessionID      `json:"sessionId"`
		Prompt            []acpschema.ContentBlock `json:"prompt"`
		WaitForCompletion bool                     `json:"waitForCompletion"`
	}{
		SessionID:         prompt.SessionID,
		Prompt:            prompt.Prompt,
		WaitForCompletion: true,
	})
	if err != nil {
		m.failPromptCall(done, job, err)
		return
	}
	var response struct {
		Outcome    string             `json:"outcome"`
		StopReason string             `json:"stopReason"`
		Meta       sessionFailureMeta `json:"_meta"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		m.failPromptCall(done, job, err)
		return
	}
	if (response.Outcome != "injected" && response.Outcome != "startedNewTurn") || response.StopReason == "" {
		m.failPromptCall(done, job, fmt.Errorf("invalid native ACP steering response"))
		return
	}
	m.completePromptCall(done, job, response.StopReason, response.Meta.terminal())
}

func (m *Manager) runSteerCallAfterHandoff(ctx context.Context, job *jobState, slot steerSlot, handoff <-chan struct{}, method steerMethod, prompt acpschema.PromptRequest) {
	done := slot.done
	for _, wait := range []<-chan struct{}{handoff, slot.after} {
		if wait == nil {
			continue
		}
		select {
		case <-wait:
		case <-done:
			return
		}
	}
	select {
	case <-done:
		return
	default:
	}
	ctx = whenSent(ctx, slot.sent)
	switch method {
	case steerPromptQueueing:
		m.runPromptCall(ctx, job, done, prompt)
	case steerNative:
		m.runNativeSteerCall(ctx, job, done, prompt)
	case steerGrokInterject:
		m.runGrokInterject(ctx, job, done, prompt)
	}
}

// answering is what sender's running turn answers, when it answers another
// bot; the result of work it starts goes on to answer it.
func (m *Manager) answering(sender string) (*storage.TurnOutput, error) {
	if sender == "" {
		return nil, nil
	}
	session, err := m.store.LoadSession(sender)
	if err != nil || session.Turn == nil {
		return nil, err
	}
	return session.Turn.Output.Continuation(), nil
}
