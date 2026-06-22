package acp

import (
	"context"
	"fmt"
	"strings"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	codexMetaKey         = "codex"
	codexSideChatMetaKey = "sideChat"
	sideChatCommand      = "side"
)

type SideChatRequest struct {
	Session string
	ID      string
	Message string
}

type sideChatScope struct {
	ID              string
	Command         string
	ParentSessionID string
	ThreadID        string
}

func (m *Manager) SendSideChat(ctx context.Context, req SideChatRequest) error {
	sideChatID := strings.TrimSpace(req.ID)
	message := strings.TrimSpace(req.Message)
	if sideChatID == "" {
		return fmt.Errorf("side chat id is required")
	}
	if message == "" {
		return fmt.Errorf("message is required")
	}
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
			return err
		}
	}
	if CanonicalAgentName(job.ACPAgent) != AgentCodex {
		return fmt.Errorf("side chat requires a codex acp session")
	}
	peer := m.peer(job.ID)
	if peer == nil {
		return fmt.Errorf("acp peer is not active")
	}
	prompt, err := promptContentBlocks(message, nil, attachmentResourceResolver{})
	if err != nil {
		return err
	}
	scope := sideChatScope{ID: sideChatID, Command: sideChatCommand, ParentSessionID: job.ID}
	m.publishSideChatMessage(job.Snapshot(), scope, "user", message, "")

	raw, err := peer.Call(ctx, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{
		SessionID: acpschema.SessionID(job.ACPSession),
		Prompt:    prompt,
		Meta:      sideChatMeta(scope),
	})
	if err != nil {
		m.publishSideChatMessage(job.Snapshot(), scope, "error", err.Error(), "error")
		return err
	}
	m.recordRawUsage(job, raw)
	return nil
}

func (m *Manager) applySideChatUpdate(job *Job, update acpschema.DecodedSessionUpdate) bool {
	switch event := update.(type) {
	case acpschema.AgentMessageChunkUpdate:
		scope, ok := sideChatScopeFromMeta(event.Meta, job.ID)
		if !ok {
			return false
		}
		m.publishSideChatMessage(job.Snapshot(), scope, "assistant", contentText(event.Content), "running")
		return true
	case acpschema.AgentThoughtChunkUpdate:
		scope, ok := sideChatScopeFromMeta(event.Meta, job.ID)
		if !ok {
			return false
		}
		m.publishSideChatMessage(job.Snapshot(), scope, "thought", contentText(event.Content), "running")
		return true
	case acpschema.ToolCallSessionUpdate:
		scope, ok := sideChatScopeFromMeta(event.Meta, job.ID)
		if !ok {
			return false
		}
		status := ""
		if event.Status != nil {
			status = string(*event.Status)
		}
		m.publishSideChatMessage(job.Snapshot(), scope, "tool", firstNonEmpty(event.Title, string(event.ToolCallID)), status)
		return true
	case acpschema.ToolCallUpdateSessionUpdate:
		scope, ok := sideChatScopeFromMeta(event.Meta, job.ID)
		if !ok {
			return false
		}
		status := ""
		if event.Status != nil {
			status = string(*event.Status)
		}
		m.publishSideChatMessage(job.Snapshot(), scope, "tool", firstNonEmpty(event.Title, string(event.ToolCallID)), status)
		return true
	default:
		return false
	}
}

func (m *Manager) publishSideChatMessage(job Job, scope sideChatScope, role, content, status string) {
	if strings.TrimSpace(scope.ID) == "" {
		return
	}
	if content == "" && status == "" {
		return
	}
	if m.Events == nil {
		return
	}
	m.Events.Publish(sessionevents.Event{
		SessionID: job.ID,
		Type:      sessionevents.TypeSideChatMessage,
		SideChat: &sessionevents.SideChatEvent{
			ID:              scope.ID,
			Command:         firstNonEmpty(scope.Command, sideChatCommand),
			ParentSessionID: firstNonEmpty(scope.ParentSessionID, job.ID),
			ThreadID:        scope.ThreadID,
			Role:            role,
			Content:         content,
			Status:          status,
		},
		At: time.Now().UTC(),
	})
}

func sideChatMeta(scope sideChatScope) map[string]any {
	return map[string]any{
		codexMetaKey: map[string]any{
			codexSideChatMetaKey: map[string]any{
				"id":              scope.ID,
				"command":         firstNonEmpty(scope.Command, sideChatCommand),
				"parentSessionId": scope.ParentSessionID,
			},
		},
	}
}

func sideChatScopeFromMeta(meta map[string]any, parentFallback string) (sideChatScope, bool) {
	codex, ok := meta[codexMetaKey].(map[string]any)
	if !ok {
		return sideChatScope{}, false
	}
	sideChat, ok := codex[codexSideChatMetaKey].(map[string]any)
	if !ok {
		return sideChatScope{}, false
	}
	id := strings.TrimSpace(stringValue(sideChat["id"]))
	if id == "" {
		return sideChatScope{}, false
	}
	return sideChatScope{
		ID:              id,
		Command:         firstNonEmpty(strings.TrimSpace(stringValue(sideChat["command"])), sideChatCommand),
		ParentSessionID: firstNonEmpty(strings.TrimSpace(stringValue(sideChat["parentSessionId"])), parentFallback),
		ThreadID:        strings.TrimSpace(stringValue(sideChat["threadId"])),
	}, true
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
