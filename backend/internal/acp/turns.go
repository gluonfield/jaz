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
	"github.com/wins/jaz/backend/internal/filepathx"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func (m *Manager) runPrompt(ctx context.Context, job *jobState, message string, attachments []storage.Attachment) {
	job.turnMu.Lock()
	defer job.turnMu.Unlock()

	done := job.turnDone()
	if done == nil {
		done = job.startTurn(CompletionInline, false, false)
	}

	peer := m.peer(job.ID)
	if peer == nil {
		m.failTurn(job, fmt.Errorf("acp peer is not active"))
		m.finishTurn(done, job)
		return
	}
	resolver := localAttachmentResources
	var err error
	if len(attachments) > 0 {
		resolver, err = m.attachmentResourceResolver(job)
		if err != nil {
			m.failTurn(job, err)
			m.finishTurn(done, job)
			return
		}
	}
	goalRequested := currentTurnGoalRequested(job, done)
	prompt, err := promptContentBlocks(message, attachments, resolver)
	if err != nil {
		m.failPromptCall(done, job, err)
		return
	}
	m.runPromptCall(ctx, job, done, acpschema.PromptRequest{
		SessionID: acpschema.SessionID(job.ACPSession),
		Prompt:    prompt,
		Meta:      goalPromptMeta(goalRequested),
	})
}

func (m *Manager) runPromptCall(ctx context.Context, job *jobState, done chan struct{}, req acpschema.PromptRequest) {
	peer := m.peer(job.ID)
	if peer == nil {
		m.failPromptCall(done, job, fmt.Errorf("acp peer is not active"))
		return
	}
	raw, err := peer.Call(ctx, acpschema.AgentMethodSessionPrompt, req)
	if err != nil {
		m.failPromptCall(done, job, err)
		return
	}
	var resp struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		m.failPromptCall(done, job, err)
		return
	}
	m.recordRawUsage(job, raw)
	m.completePromptCall(done, job, resp.StopReason)
}

func (m *Manager) completePromptCall(done chan struct{}, job *jobState, stopReason string) {
	state := StateIdle
	if jobCancelRequested(job) || stopReason == "cancelled" {
		state = StateCancelled
		stopReason = "cancelled"
	}
	job.mu.Lock()
	turn := job.turn
	if turn == nil || turn.done != done {
		job.mu.Unlock()
		return
	}
	if turn.promptCalls > 0 {
		turn.promptCalls--
	}
	remaining := turn.promptCalls
	if state == StateIdle && remaining > 0 {
		job.UpdatedAt = time.Now().UTC()
		job.mu.Unlock()
		m.log.Info("acp prompt handed off", "session", job.ID, "remaining_prompt_calls", remaining)
		return
	}
	turn.promptCalls = 0
	job.State = state
	job.StopReason = stopReason
	job.Error = ""
	job.UpdatedAt = time.Now().UTC()
	job.mu.Unlock()
	m.log.Info("acp turn finished", "session", job.ID, "state", state, "stop_reason", stopReason)
	m.publishACPStatus(job.Snapshot())
	m.appendAssistantMessage(job)
	m.finishTurn(done, job)
}

func (m *Manager) failPromptCall(done chan struct{}, job *jobState, err error) {
	job.mu.Lock()
	turn := job.turn
	if turn == nil || turn.done != done {
		job.mu.Unlock()
		return
	}
	if turn.promptCalls > 0 {
		turn.promptCalls--
	}
	turn.promptCalls = 0
	job.mu.Unlock()
	m.failTurn(job, err)
	m.finishTurn(done, job)
}

func (m *Manager) finishTurn(done chan struct{}, job *jobState) {
	job.mu.Lock()
	turn := job.turn
	if turn == nil || turn.done != done {
		job.mu.Unlock()
		return
	}
	turn.closeFirstPromptSent()
	job.turn = nil
	completion := turn.completion
	planRequested := turn.planRequested
	parentVisible := job.ParentVisible
	job.mu.Unlock()
	m.cancelPendingPermissions(job.ID)
	m.resolveDanglingToolCalls(job)
	snapshot := job.Snapshot()
	if snapshot.State == StateIdle || snapshot.State == StateFailed || snapshot.State == StateCancelled {
		if planRequested && snapshot.State == StateIdle {
			m.publishPlanTurnResult(snapshot)
		}
		m.compactSessionEvents(snapshot.ID)
		m.touchAttention(surfaceSessionIDs(&snapshot)...)
	}
	if m.TurnFinished != nil {
		m.TurnFinished(context.Background(), snapshot)
	}
	if done != nil {
		close(done)
	}
	if completion.propagates() && parentVisible && !planRequested && m.Done != nil {
		go m.Done(context.Background(), snapshot)
	}
}

func jobCancelRequested(job *jobState) bool {
	job.mu.RLock()
	cancelled := job.turn != nil && job.turn.cancelRequested
	job.mu.RUnlock()
	return cancelled
}

func (m *Manager) attachmentResourceResolver(job *jobState) (attachmentResourceResolver, error) {
	cfg, ok, err := m.configuredAgent(job.ACPAgent)
	if err != nil {
		return attachmentResourceResolver{}, err
	}
	if !ok {
		return attachmentResourceResolver{}, fmt.Errorf("acp agent %q is not configured", job.ACPAgent)
	}
	return attachmentResourceResolver{localFiles: strings.TrimSpace(cfg.URL) == ""}, nil
}

type attachmentResourceResolver struct {
	localFiles bool
}

var localAttachmentResources = attachmentResourceResolver{localFiles: true}

func (r attachmentResourceResolver) URI(attachment storage.Attachment) (string, error) {
	serverPath := strings.TrimSpace(attachment.ServerPath)
	if serverPath != "" {
		if r.localFiles {
			return filepathx.FileURI(serverPath), nil
		}
		if uri := strings.TrimSpace(attachment.URI); uri != "" && !strings.HasPrefix(strings.ToLower(uri), "file:") {
			return uri, nil
		}
		return "", fmt.Errorf("attachment %q is server-local; remote ACP attachment resources are not supported yet", attachment.Name)
	}
	if uri := strings.TrimSpace(attachment.URI); uri != "" {
		return uri, nil
	}
	return "", fmt.Errorf("attachment %q has no resource URI", attachment.Name)
}

func promptContentBlocks(message string, attachments []storage.Attachment, resolver attachmentResourceResolver) ([]acpschema.ContentBlock, error) {
	out := make([]acpschema.ContentBlock, 0, 1+len(attachments))
	var err error
	out, err = appendTextBlock(out, message)
	if err != nil {
		return nil, err
	}
	for _, attachment := range attachments {
		uri, err := resolver.URI(attachment)
		if err != nil {
			return nil, err
		}
		block := acpschema.ResourceLinkContentBlock{
			Kind: acpschema.ContentBlockResourceLink,
			ResourceLink: acpschema.ResourceLink{
				Name:     attachment.Name,
				URI:      uri,
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
func (m *Manager) failTurn(job *jobState, err error) {
	if jobCancelRequested(job) {
		job.setState(StateCancelled, "cancelled", "")
		m.log.Info("acp turn cancelled", "session", job.ID)
	} else {
		if serveErr := m.serveErr(job.ID); serveErr != nil && errors.Is(err, jsonrpc.ErrClosed) {
			err = serveErr
		}
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

func (m *Manager) compactSessionEvents(sessionID string) {
	compactor, ok := m.store.(storage.SessionEventCompactor)
	if !ok {
		return
	}
	removed, err := compactor.CompactSessionEvents(sessionID)
	if err != nil {
		m.log.Error("compact session events failed", "session", sessionID, "error", err)
		return
	}
	if removed > 0 {
		m.log.Debug("compacted session events", "session", sessionID, "removed", removed)
	}
}

// A cancelled or failed turn leaves the agent's in-flight tool calls without
// terminal updates; resolve them so they don't render as running forever.
func (m *Manager) resolveDanglingToolCalls(job *jobState) {
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
	now := time.Now().UTC()
	var updated []sessionevents.ACPToolCall
	for id, call := range job.toolByID {
		if terminalToolStatus(call.Status) {
			continue
		}
		call.Status = status
		call.UpdatedAt = now
		job.toolByID[id] = call
		updated = append(updated, call)
	}
	if len(updated) == 0 {
		job.mu.Unlock()
		return
	}
	job.ToolCalls = sortedToolCalls(job.toolByID)
	job.UpdatedAt = now
	job.LastEventAt = now
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

func (m *Manager) appendAssistantMessage(job *jobState) {
	job.mu.Lock()
	if job.turn != nil && job.turn.planRequested {
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
