package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

func (m *Manager) runPrompt(ctx context.Context, job *Job, message string, attachments []storage.Attachment) {
	job.turnMu.Lock()
	defer job.turnMu.Unlock()

	job.mu.RLock()
	done := job.done
	job.mu.RUnlock()
	if done == nil {
		done = job.startTurn(CompletionInline, false, false, false)
	}

	peer := m.peer(job.ID)
	if peer == nil {
		m.failTurn(job, fmt.Errorf("acp peer is not active"))
		m.finishTurn(done, job)
		return
	}
	prompt, err := promptContentBlocks(message, attachments)
	if err != nil {
		m.failTurn(job, err)
		m.finishTurn(done, job)
		return
	}
	raw, err := peer.Call(ctx, acpschema.AgentMethodSessionPrompt, map[string]any{
		"sessionId": job.ACPSession,
		"prompt":    prompt,
	})
	if err != nil {
		m.failTurn(job, err)
		m.finishTurn(done, job)
		return
	}
	var resp struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		m.failTurn(job, err)
		m.finishTurn(done, job)
		return
	}
	if usage := usageFromRaw(raw); !usageEmpty(usage) {
		m.recordUsage(job, usage)
	}
	stopReason := resp.StopReason
	state := StateIdle
	if stopReason == "cancelled" {
		state = StateCancelled
	}
	job.setState(state, stopReason, "")
	m.log.Info("acp turn finished", "session", job.ID, "state", state, "stop_reason", stopReason)
	m.publishACPStatus(job.Snapshot())
	time.Sleep(50 * time.Millisecond)
	m.persistUsage(job)
	m.appendAssistantMessage(job)
	m.finishTurn(done, job)
}

func promptContentBlocks(message string, attachments []storage.Attachment) ([]acpschema.ContentBlock, error) {
	out := make([]acpschema.ContentBlock, 0, 1+len(attachments))
	text, err := marshalContentBlock(map[string]any{"type": "text", "text": message})
	if err != nil {
		return nil, err
	}
	out = append(out, text)
	for _, attachment := range attachments {
		block := acpschema.ResourceLinkContentBlock{
			Kind: acpschema.ContentBlockResourceLink,
			ResourceLink: acpschema.ResourceLink{
				Name:     attachment.Name,
				URI:      attachment.URI,
				MimeType: attachment.MimeType,
				Size:     attachment.Size,
			},
		}
		raw, err := marshalContentBlock(block)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

func marshalContentBlock(block any) (acpschema.ContentBlock, error) {
	data, err := json.Marshal(block)
	if err != nil {
		return nil, err
	}
	return acpschema.ContentBlock(data), nil
}

// A turn that died after a cancel request ends as cancelled, not failed; both
// outcomes are published so the UI and stored status reflect them.
func (m *Manager) failTurn(job *Job, err error) {
	job.mu.RLock()
	cancelled := job.cancelRequested
	job.mu.RUnlock()
	if cancelled {
		job.setState(StateCancelled, "cancelled", "")
		m.log.Info("acp turn cancelled", "session", job.ID)
	} else {
		job.setState(StateFailed, "", err.Error())
		m.log.Error("acp turn failed", "session", job.ID, "error", err)
	}
	m.publishACPStatus(job.Snapshot())
}

func (m *Manager) finishTurn(done chan struct{}, job *Job) {
	job.mu.Lock()
	completion := job.completion
	planRequested := job.planRequested
	parentVisible := job.ParentVisible
	job.completion = CompletionInline
	job.interactive = false
	job.planRequested = false
	job.mu.Unlock()
	m.cancelPendingPermissions(job.ID)
	m.resolveDanglingToolCalls(job)
	snapshot := job.Snapshot()
	if m.TurnFinished != nil {
		m.TurnFinished(context.Background(), snapshot)
	}
	close(done)
	if completion.propagates() && parentVisible && !planRequested && m.Done != nil {
		go m.Done(context.Background(), snapshot)
	}
}

// A cancelled or failed turn leaves the agent's in-flight tool calls without
// terminal updates; resolve them so they don't render as running forever.
func (m *Manager) resolveDanglingToolCalls(job *Job) {
	job.mu.Lock()
	state := job.State
	if state != StateCancelled && state != StateFailed {
		job.mu.Unlock()
		return
	}
	status := "cancelled"
	if state == StateFailed {
		status = "failed"
	}
	var updated []ToolCallSnapshot
	for id, call := range job.toolByID {
		if terminalToolStatus(call.Status) {
			continue
		}
		call.Status = status
		job.toolByID[id] = call
		updated = append(updated, call)
	}
	if len(updated) == 0 {
		job.mu.Unlock()
		return
	}
	job.ToolCalls = sortedToolCalls(job.toolByID)
	job.UpdatedAt = time.Now().UTC()
	sessionID := job.ID
	job.mu.Unlock()
	m.log.Info("resolved dangling tool calls", "session", sessionID, "count", len(updated), "status", status)
	for _, call := range updated {
		_ = m.store.UpsertActivity(sessionID, storage.ActivityEntry{
			ID:     call.ID,
			Kind:   "tool",
			Text:   firstNonEmpty(call.Title, call.ID),
			Status: call.Status,
			At:     time.Now().UTC(),
		})
		m.publishACPTool(job.Snapshot(), call)
	}
}

func terminalToolStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "failed", "cancelled", "canceled":
		return true
	}
	return false
}

func (m *Manager) appendAssistantMessage(job *Job) {
	job.mu.Lock()
	if job.planRequested {
		job.mu.Unlock()
		return
	}
	if job.savedAssistantLen >= len(job.Assistant) {
		job.mu.Unlock()
		return
	}
	content := job.Assistant[job.savedAssistantLen:]
	job.savedAssistantLen = len(job.Assistant)
	sessionID := job.ID
	job.mu.Unlock()
	if strings.TrimSpace(content) == "" {
		return
	}
	_ = m.store.AppendMessages(sessionID, provider.AssistantMessage(content, nil))
}
