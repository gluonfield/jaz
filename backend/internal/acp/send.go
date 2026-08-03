package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

var ErrSteeringUnsupported = errors.New("acp steering unsupported")

type sendTranscriptMode int

const (
	sendTranscriptUserMessage sendTranscriptMode = iota
	sendTranscriptHidden
)

type sendOptions struct {
	activeOperation       string
	transcript            sendTranscriptMode
	requireCompactSupport bool
}

type InternalTurnRequest struct {
	Session string
	Message string
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
	}, sendOptions{transcript: sendTranscriptHidden})
}

func (m *Manager) StartInternalTurn(ctx context.Context, req InternalTurnRequest) (Job, error) {
	return m.send(ctx, SendRequest{
		Session:    req.Session,
		Message:    req.Message,
		Completion: CompletionAsync,
	}, sendOptions{transcript: sendTranscriptHidden})
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
	if opts.requireCompactSupport && !AgentSupportsCompact(job.ACPAgent) {
		return Job{}, fmt.Errorf("compact is not available for acp agent %q", job.ACPAgent)
	}
	local := m.localAgent(job.ACPAgent)
	if m.configuredLocal(job.ACPAgent) && local == nil {
		return Job{}, fmt.Errorf("local acp agent %q is not registered", job.ACPAgent)
	}
	job.sendMu.Lock()
	if job.turnDone() != nil {
		job.sendMu.Unlock()
		return Job{}, fmt.Errorf("session %s is already running", job.Slug)
	}
	job.sendMu.Unlock()
	var processLease *processLease
	if local == nil {
		job, processLease, err = m.acquireSessionProcess(ctx, job)
		if err != nil {
			return Job{}, err
		}
	}
	job.sendMu.Lock()
	defer job.sendMu.Unlock()
	started := false
	defer func() {
		if !started {
			processLease.Release()
		}
	}()
	if job.turnDone() != nil {
		return Job{}, fmt.Errorf("session %s is already running", job.Slug)
	}
	if err := m.prepareModeForTurn(ctx, job, req.PlanRequested); err != nil {
		return Job{}, err
	}
	promptMessage, contexts := promptMessageAndContexts(req.Message, req.Contexts)
	var prompt acpschema.PromptRequest
	if local == nil {
		prompt, err = m.promptRequest(job, goalPromptMessage(promptMessage, req.GoalRequested), req.Attachments)
		if err != nil {
			return Job{}, err
		}
	}
	if err := m.store.UpdateSessionStatus(job.ID, storage.StatusRunning, "", time.Now().UTC()); err != nil {
		return Job{}, fmt.Errorf("mark session running: %w", err)
	}
	if opts.transcript == sendTranscriptUserMessage {
		if err := storage.AppendUserMessage(m.store, job.ID, req.Message, contexts, req.Attachments); err != nil {
			appendErr := fmt.Errorf("append user message: %w", err)
			if rollbackErr := m.store.UpdateSessionStatus(job.ID, storage.StatusIdle, "", time.Now().UTC()); rollbackErr != nil {
				return Job{}, errors.Join(appendErr, fmt.Errorf("restore session idle: %w", rollbackErr))
			}
			return Job{}, appendErr
		}
	}
	m.log.Info("acp turn started", "session", job.ID, "agent", job.ACPAgent, "plan", req.PlanRequested, "goal", req.GoalRequested, "operation", opts.activeOperation)
	job.startTurnWithOperation(req.Completion, req.PlanRequested, req.ParentVisible, opts.activeOperation)
	job.mu.Lock()
	job.turn.processLease = processLease
	job.mu.Unlock()
	started = true
	m.touchAttention(parentSessionIDs(job.eventView())...)
	markGoalRequested(job, req.GoalRequested)
	m.publishACP(job.eventView())
	if local != nil {
		go m.runLocalPrompt(context.WithoutCancel(ctx), job, local, promptMessage, req.Attachments)
	} else {
		go m.runPrompt(context.Background(), job, prompt)
	}
	return job.Snapshot(), nil
}

func (m *Manager) Steer(ctx context.Context, req SteerRequest) (Job, error) {
	job, err := m.job(req.Session)
	if err != nil {
		return Job{}, err
	}
	if !storage.HasMessageContent(req.Message, req.Contexts, req.Attachments) {
		return Job{}, fmt.Errorf("message is required")
	}
	job.mu.RLock()
	method := job.steerMethod
	job.mu.RUnlock()
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
	promptMessage, contexts := promptMessageAndContexts(req.Message, req.Contexts)
	promptReq, err := m.promptRequest(job, goalPromptMessage(promptMessage, req.GoalRequested), req.Attachments)
	if err != nil {
		return Job{}, err
	}
	done, err := m.reserveSteer(job, req, contexts)
	if err != nil {
		return Job{}, err
	}
	handoff := m.cancelPendingPermissionsForSteer(job, done)
	m.touchJobAttention(job)
	markGoalRequested(job, req.GoalRequested)
	m.publishACP(job.eventView())
	go m.runSteerCallAfterHandoff(context.Background(), job, done, handoff, method, promptReq)
	return job.Snapshot(), nil
}

func (m *Manager) reserveSteer(job *jobState, req SteerRequest, contexts []storage.MessageContext) (chan struct{}, error) {
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.steerMethod == steerUnsupported || job.turn == nil || (job.State != StateRunning && job.State != StateStarting) {
		return nil, ErrSteeringUnsupported
	}
	if err := storage.AppendUserMessage(m.store, job.ID, req.Message, contexts, req.Attachments); err != nil {
		return nil, fmt.Errorf("append user message: %w", err)
	}
	if req.ParentVisible {
		job.ParentVisible = true
	}
	job.turn.promptCalls++
	return job.turn.done, nil
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
		Outcome    string `json:"outcome"`
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		m.failPromptCall(done, job, err)
		return
	}
	if (response.Outcome != "injected" && response.Outcome != "startedNewTurn") || response.StopReason == "" {
		m.failPromptCall(done, job, fmt.Errorf("invalid native ACP steering response"))
		return
	}
	m.completePromptCall(done, job, response.StopReason)
}

func (m *Manager) runSteerCallAfterHandoff(ctx context.Context, job *jobState, done chan struct{}, handoff <-chan struct{}, method steerMethod, prompt acpschema.PromptRequest) {
	if handoff != nil {
		select {
		case <-handoff:
		case <-done:
			return
		}
	}
	select {
	case <-done:
		return
	default:
	}
	switch method {
	case steerPromptQueueing:
		m.runPromptCall(ctx, job, done, prompt)
	case steerNative:
		m.runNativeSteerCall(ctx, job, done, prompt)
	}
}
