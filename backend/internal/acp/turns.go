package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
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
	context, err := m.turnPromptContext(message)
	if err != nil {
		m.failTurn(job, err)
		m.finishTurn(done, job)
		return
	}
	prompt, err := promptContentBlocks(context, message, attachments)
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
	if usage := usageFromRaw(raw); !usage.IsZero() {
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

func (m *Manager) turnPromptContext(message string) (string, error) {
	if !strings.Contains(message, "$") || m.cfg.SystemPrompt == nil {
		return "", nil
	}
	prompt, err := m.cfg.SystemPrompt.SkillsPrompt()
	if err != nil {
		return "", fmt.Errorf("build acp skills prompt: %w", err)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", nil
	}
	return "Current skills catalog for this turn. Use it to resolve any $skill references in the user's message.\n\n" + prompt, nil
}

func promptContentBlocks(context, message string, attachments []storage.Attachment) ([]acpschema.ContentBlock, error) {
	out := make([]acpschema.ContentBlock, 0, 2+len(attachments))
	if strings.TrimSpace(context) != "" {
		var err error
		out, err = appendTextBlock(out, context)
		if err != nil {
			return nil, err
		}
	}
	var err error
	out, err = appendTextBlock(out, message)
	if err != nil {
		return nil, err
	}
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

func appendTextBlock(out []acpschema.ContentBlock, text string) ([]acpschema.ContentBlock, error) {
	block, err := marshalContentBlock(acpschema.TextContentBlock{
		Kind:        acpschema.ContentBlockText,
		TextContent: acpschema.TextContent{Text: text},
	})
	if err != nil {
		return nil, err
	}
	return append(out, block), nil
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
		message := acpTurnErrorMessage(err)
		job.setState(StateFailed, "", message)
		m.log.Error("acp turn failed", "session", job.ID, "error", err)
	}
	m.publishACPStatus(job.Snapshot())
}

func acpTurnErrorMessage(err error) string {
	var rpcErr *jsonrpc.Error
	if errors.As(err, &rpcErr) {
		if message := jsonRPCErrorDataMessage(rpcErr.Data); message != "" {
			return message
		}
		if message := strings.TrimSpace(rpcErr.Message); message != "" && message != "Internal error" {
			return message
		}
	}
	return err.Error()
}

func jsonRPCErrorDataMessage(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var data struct {
		Message json.RawMessage `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &data) == nil && (len(data.Message) > 0 || len(data.Error) > 0) {
		if message := jsonErrorMessage(data.Message); message != "" {
			return message
		}
		if message := jsonErrorMessage(data.Error); message != "" {
			return message
		}
	}
	return jsonErrorMessage(raw)
}

func jsonErrorMessage(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		text = strings.TrimSpace(text)
		if nested := jsonErrorMessage(json.RawMessage(text)); nested != "" {
			return nested
		}
		return text
	}
	if raw[0] != '{' {
		return ""
	}
	var payload struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	if message := strings.TrimSpace(payload.Error.Message); message != "" {
		return message
	}
	return strings.TrimSpace(payload.Message)
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
	if snapshot.State == StateIdle || snapshot.State == StateFailed || snapshot.State == StateCancelled {
		m.touchAttention(surfaceSessionIDs(&snapshot)...)
	}
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
