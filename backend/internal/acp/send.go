package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

var ErrPromptQueueingUnsupported = errors.New("acp prompt queueing unsupported")

func (m *Manager) Send(ctx context.Context, req SendRequest) (Job, error) {
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
	if strings.TrimSpace(req.Message) == "" {
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
	if !req.SkipUserMessage {
		if err := storage.AppendUserMessage(m.store, job.ID, req.Message, contexts, req.Attachments); err != nil {
			m.log.Error("append user message failed", "session", job.ID, "error", err)
		}
	}
	m.log.Info("acp turn started", "session", job.ID, "agent", job.ACPAgent, "plan", req.PlanRequested)
	job.startTurnWithOperation(req.Completion, req.PlanRequested, req.ParentVisible, req.ActiveOperation)
	m.touchJobAttention(job)
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
	if strings.TrimSpace(req.Message) == "" {
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
	prompt, err := promptContentBlocks(promptMessage, req.Attachments, resolver)
	if err != nil {
		return Job{}, err
	}
	done, ok := job.addPromptCall(req.ParentVisible)
	if !ok {
		return Job{}, ErrPromptQueueingUnsupported
	}
	if err := storage.AppendUserMessage(m.store, job.ID, req.Message, contexts, req.Attachments); err != nil {
		m.log.Error("append user message failed", "session", job.ID, "error", err)
	}
	m.touchJobAttention(job)
	m.publishACP(job.Snapshot())
	go m.runPromptCall(context.Background(), job, done, acpschema.PromptRequest{
		SessionID: acpschema.SessionID(job.ACPSession),
		Prompt:    prompt,
	})
	return job.Snapshot(), nil
}
