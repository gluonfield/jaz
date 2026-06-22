package acp

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

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
	contexts := storage.NormalizeMessageContexts(req.Contexts)
	if err := storage.AppendUserMessage(m.store, job.ID, req.Message, contexts, req.Attachments); err != nil {
		m.log.Error("append user message failed", "session", job.ID, "error", err)
	}
	promptMessage := messageWithContext(req.Message, contexts)
	m.log.Info("acp turn started", "session", job.ID, "agent", job.ACPAgent, "plan", req.PlanRequested)
	job.startTurn(req.Completion, req.Interactive, req.PlanRequested, req.ParentVisible)
	m.touchJobAttention(job)
	m.publishACP(job.Snapshot())
	if local != nil {
		go m.runLocalPrompt(context.WithoutCancel(ctx), job, local, promptMessage, req.Attachments)
	} else {
		go m.runPrompt(context.Background(), job, promptMessage, req.Attachments)
	}
	return job.Snapshot(), nil
}
