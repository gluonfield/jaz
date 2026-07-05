package acp

import (
	"context"
	"errors"
	"fmt"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

var ErrPromptQueueingUnsupported = errors.New("acp prompt queueing unsupported")

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
	job, err := m.job(req.Session)
	if err == nil && m.serveErr(job.ID) != nil {
		job.mu.RLock()
		running := job.State == StateRunning || job.State == StateStarting
		job.mu.RUnlock()
		if !running {
			m.teardown(job.ID)
			job, err = m.resume(ctx, req.Session)
		}
	}
	if err != nil {
		if job, err = m.resume(ctx, req.Session); err != nil {
			return Job{}, err
		}
	}
	if opts.requireCompactSupport && !AgentSupportsCompact(job.ACPAgent) {
		return Job{}, fmt.Errorf("compact is not available for acp agent %q", job.ACPAgent)
	}
	if !storage.HasMessageContent(req.Message, req.Contexts, req.Attachments) {
		return Job{}, fmt.Errorf("message is required")
	}
	job.mu.RLock()
	state := job.State
	job.mu.RUnlock()
	if state == StateRunning || state == StateStarting {
		return Job{}, fmt.Errorf("session %s is already running", job.Slug)
	}
	local := m.localAgent(job.ACPAgent)
	if m.configuredLocal(job.ACPAgent) && local == nil {
		return Job{}, fmt.Errorf("local acp agent %q is not registered", job.ACPAgent)
	}
	if err := m.prepareModeForTurn(ctx, job, req.PlanRequested); err != nil {
		return Job{}, err
	}
	promptMessage, contexts := promptMessageAndContexts(req.Message, req.Contexts)
	if opts.transcript == sendTranscriptUserMessage {
		if err := storage.AppendUserMessage(m.store, job.ID, req.Message, contexts, req.Attachments); err != nil {
			m.log.Error("append user message failed", "session", job.ID, "error", err)
		}
	}
	m.log.Info("acp turn started", "session", job.ID, "agent", job.ACPAgent, "plan", req.PlanRequested, "goal", req.GoalRequested, "operation", opts.activeOperation)
	job.startTurnWithOperation(req.Completion, req.PlanRequested, req.ParentVisible, opts.activeOperation)
	m.touchJobAttention(job)
	markGoalRequested(job, req.GoalRequested)
	m.publishACP(job.Snapshot())
	if local != nil {
		go m.runLocalPrompt(context.WithoutCancel(ctx), job, local, promptMessage, req.Attachments)
	} else {
		go m.runPrompt(context.Background(), job, promptMessage, req.Attachments)
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
	queueing := job.promptQueueing
	job.mu.RUnlock()
	if !queueing {
		return Job{}, ErrPromptQueueingUnsupported
	}
	local := m.localAgent(job.ACPAgent)
	if m.configuredLocal(job.ACPAgent) && local == nil {
		return Job{}, fmt.Errorf("local acp agent %q is not registered", job.ACPAgent)
	}
	if local != nil {
		return Job{}, ErrPromptQueueingUnsupported
	}
	if err := job.waitFirstPromptSent(ctx); err != nil {
		return Job{}, err
	}
	peer := m.peer(job.ID)
	if peer == nil {
		return Job{}, fmt.Errorf("acp peer is not active")
	}
	resolver := localAttachmentResources
	if len(req.Attachments) > 0 {
		resolver, err = m.attachmentResourceResolver(job)
		if err != nil {
			return Job{}, err
		}
	}
	promptMessage, contexts := promptMessageAndContexts(req.Message, req.Contexts)
	prompt, err := promptContentBlocks(goalPromptMessage(promptMessage, req.GoalRequested), req.Attachments, resolver)
	if err != nil {
		return Job{}, err
	}
	promptReq := acpschema.PromptRequest{
		SessionID: acpschema.SessionID(job.ACPSession),
		Prompt:    prompt,
	}
	done, ok := job.addPromptCall(req.ParentVisible)
	if !ok {
		return Job{}, ErrPromptQueueingUnsupported
	}
	handoff := m.cancelPendingPermissionsForSteer(job, done)
	if err := storage.AppendUserMessage(m.store, job.ID, req.Message, contexts, req.Attachments); err != nil {
		m.log.Error("append user message failed", "session", job.ID, "error", err)
	}
	m.touchJobAttention(job)
	markGoalRequested(job, req.GoalRequested)
	m.publishACP(job.Snapshot())
	go m.runPromptCallAfterHandoff(context.Background(), job, done, handoff, promptReq)
	return job.Snapshot(), nil
}

func (m *Manager) runPromptCallAfterHandoff(ctx context.Context, job *jobState, done chan struct{}, handoff <-chan struct{}, req acpschema.PromptRequest) {
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
	m.runPromptCall(ctx, job, done, req)
}
